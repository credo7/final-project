package handlers

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"project_sem/storage"
)

type ResponseData struct {
	TotalItems      int     `json:"total_items"`
	TotalCategories int     `json:"total_categories"`
	TotalPrice      float64 `json:"total_price"`
}

func PostPricesHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("Entering PostPricesHandler")

	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		log.Printf("Error reading file from form: %v\n", err)
		http.Error(w, "Failed to read file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	log.Printf("Successfully received file: %s\n", fileHeader.Filename)

	zipReader, err := zip.NewReader(file, r.ContentLength)
	if err != nil {
		log.Printf("Error reading ZIP archive: %v\n", err)
		http.Error(w, "Failed to read zip archive", http.StatusInternalServerError)
		return
	}
	log.Println("ZIP archive opened successfully")

	db := storage.GetDB()
	log.Println("Successfully connected to the database")

	tx, err := db.Begin()
	if err != nil {
		log.Printf("Error beginning transaction: %v\n", err)
		http.Error(w, "Failed to begin transaction", http.StatusInternalServerError)
		return
	}
	log.Println("Transaction started")

	// In case of error, rollback
	defer func() {
		if err != nil {
			log.Println("Rolling back transaction due to an error")
			tx.Rollback()
		}
	}()

	for _, zipFile := range zipReader.File {
		log.Printf("Processing file inside ZIP: %s\n", zipFile.Name)
		fileReader, err := zipFile.Open()
		if err != nil {
			log.Printf("Error reading file in ZIP: %v\n", err)
			http.Error(w, "Failed to read file in zip", http.StatusInternalServerError)
			return
		}
		defer fileReader.Close()

		csvReader := csv.NewReader(fileReader)
		log.Println("Created CSV reader")

		headerRow, err := csvReader.Read() // читаем хедер
		if err != nil {
			log.Printf("Error reading header row: %v\n", err)
			http.Error(w, "Failed to read header row", http.StatusInternalServerError)
			return
		}
		log.Printf("Header row read successfully: %v\n", headerRow)

		for {
			row, err := csvReader.Read()
			if err == io.EOF {
				log.Println("Reached end of CSV file")
				break
			}
			if err != nil {
				log.Printf("Error reading CSV row: %v\n", err)
				http.Error(w, "Failed to read CSV row", http.StatusInternalServerError)
				return
			}

			log.Printf("Read CSV row: %v\n", row)

			id, err := strconv.Atoi(row[0])
			if err != nil {
				log.Printf("Error converting ID (%s): %v\n", row[0], err)
				http.Error(w, "Invalid ID value", http.StatusBadRequest)
				return
			}
			log.Printf("Parsed ID: %d\n", id)

			itemName := row[1]
			category := row[2]

			price, err := strconv.ParseFloat(row[3], 64)
			if err != nil {
				log.Printf("Error parsing price (%s): %v\n", row[3], err)
				http.Error(w, "Invalid price value", http.StatusBadRequest)
				return
			}
			log.Printf("Parsed price: %.2f\n", price)

			createDate, err := time.Parse("2006-01-02", row[4])
			if err != nil {
				log.Printf("Error parsing date (%s): %v\n", row[4], err)
				http.Error(w, "Invalid date format", http.StatusBadRequest)
				return
			}
			log.Printf("Parsed date: %v\n", createDate)

			_, err = tx.Exec(`
				INSERT INTO prices (id, name, category, price, create_date)
				VALUES ($1, $2, $3, $4, $5)
			`, id, itemName, category, price, createDate)
			if err != nil {
				log.Printf("Error inserting data into prices table: %v\n", err)
				http.Error(w, "Failed to insert data", http.StatusInternalServerError)
				return
			}
			log.Println("Inserted row into prices table successfully")
		}
	}

	var totalItems int
	var totalCategories int
	var totalPrice float64

	err = tx.QueryRow(`
		SELECT 
			COUNT(*) AS total_items,
			COUNT(DISTINCT category) AS total_categories,
			COALESCE(SUM(price), 0) AS total_price
		FROM prices
	`).Scan(&totalItems, &totalCategories, &totalPrice)
	if err != nil {
		log.Printf("Error reading stats from database: %v\n", err)
		tx.Rollback()
		http.Error(w, "Failed to get statistics", http.StatusInternalServerError)
		return
	}

	if err = tx.Commit(); err != nil {
		log.Printf("Error committing transaction: %v\n", err)
		http.Error(w, "Failed to commit transaction", http.StatusInternalServerError)
		return
	}

	response := ResponseData{
		TotalItems:      totalItems,
		TotalCategories: totalCategories,
		TotalPrice:      totalPrice,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding JSON response: %v\n", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

func GetPricesHandler(w http.ResponseWriter, r *http.Request) {
	db := storage.GetDB()
	rows, err := db.Query("SELECT id, name, category, price, create_date FROM prices")
	if err != nil {
		log.Printf("Failed to fetch data: %v", err)
		http.Error(w, "Failed to fetch data", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	// 1. Считываем все строки из базы в слайс
	var records [][]string
	for rows.Next() {
		var (
			id         int
			name       string
			category   string
			price      float64
			createDate time.Time
		)

		if err := rows.Scan(&id, &name, &category, &price, &createDate); err != nil {
			log.Printf("Failed to read data from DB: %v", err)
			http.Error(w, "Failed to read data from DB", http.StatusInternalServerError)
			return
		}

		priceStr := strconv.FormatFloat(price, 'f', 2, 64)
		createDateStr := createDate.Format(time.RFC3339)

		record := []string{
			strconv.Itoa(id),
			name,
			category,
			priceStr,
			createDateStr,
		}
		records = append(records, record)
	}

	// 2. Проверяем, не возникли ли ошибки при чтении rows
	if err := rows.Err(); err != nil {
		log.Printf("Error during rows iteration: %v", err)
		http.Error(w, "Failed to iterate rows", http.StatusInternalServerError)
		return
	}

	// 3. Во втором цикле записываем данные в CSV
	var csvBuffer bytes.Buffer
	csvWriter := csv.NewWriter(&csvBuffer)

	for _, rec := range records {
		if err := csvWriter.Write(rec); err != nil {
			log.Printf("Failed to write record to CSV: %v", err)
			http.Error(w, "Failed to write CSV", http.StatusInternalServerError)
			return
		}
	}
	csvWriter.Flush()

	if err := csvWriter.Error(); err != nil {
		log.Printf("Error flushing CSV writer: %v", err)
		http.Error(w, "Failed to write CSV", http.StatusInternalServerError)
		return
	}

	// 4. Подготовка ответа в виде ZIP-архива
	w.Header().Set("Content-Disposition", "attachment; filename=response.zip")
	w.Header().Set("Content-Type", "application/zip")

	zipWriter := zip.NewWriter(w)
	defer zipWriter.Close()

	fileWriter, err := zipWriter.Create("data.csv")
	if err != nil {
		log.Printf("Failed to create file in zip: %v", err)
		http.Error(w, "Failed to create file in zip", http.StatusInternalServerError)
		return
	}

	if _, err := io.Copy(fileWriter, &csvBuffer); err != nil {
		log.Printf("Failed to copy data to zip: %v", err)
		http.Error(w, "Failed to copy data to zip", http.StatusInternalServerError)
		return
	}
}
