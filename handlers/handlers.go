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

	"project_sem/storage"
)

type ResponseData struct {
	TotalItems      int     `json:"total_items"`
	TotalCategories int     `json:"total_categories"`
	TotalPrice      float64 `json:"total_price"`
}

func PostPricesHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("Entering PostPricesHandler")

	file, _, err := r.FormFile("file")
	if err != nil {
		log.Printf("Error reading file from form: %v\n", err)
		http.Error(w, "Failed to read file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	zipReader, err := zip.NewReader(file, r.ContentLength)
	if err != nil {
		log.Printf("Error reading ZIP archive: %v\n", err)
		http.Error(w, "Failed to read zip archive", http.StatusInternalServerError)
		return
	}

	db := storage.GetDB()
	tx, err := db.Begin()
	if err != nil {
		log.Printf("Error beginning transaction: %v\n", err)
		http.Error(w, "Failed to begin transaction", http.StatusInternalServerError)
		return
	}

	defer func() {
		// Если где-то возникла ошибка, откатываем
		if err != nil {
			tx.Rollback()
		}
	}()

	for _, zipFile := range zipReader.File {
		fileReader, err := zipFile.Open()
		if err != nil {
			log.Printf("Error reading file in ZIP: %v\n", err)
			http.Error(w, "Failed to read file in zip", http.StatusInternalServerError)
			return
		}
		defer fileReader.Close()

		csvReader := csv.NewReader(fileReader)

		_, err = csvReader.Read() // читаем хедер
		if err != nil {
			log.Printf("Error reading header row: %v\n", err)
			http.Error(w, "Failed to read header row", http.StatusInternalServerError)
			return
		}

		for {
			row, err := csvReader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Printf("Error reading CSV row: %v\n", err)
				http.Error(w, "Failed to read CSV row", http.StatusInternalServerError)
				return
			}

			id, err := strconv.Atoi(row[0])
			if err != nil {
				log.Printf("Error converting ID (%s): %v\n", row[0], err)
				http.Error(w, "Invalid ID value", http.StatusBadRequest)
				return
			}

			itemName := row[1]
			category := row[2]

			price, err := strconv.ParseFloat(row[3], 64)
			if err != nil {
				log.Printf("Error parsing price (%s): %v\n", row[3], err)
				http.Error(w, "Invalid price value", http.StatusBadRequest)
				return
			}

			createDate, err := time.Parse(time.RFC3339, row[4])
			if err != nil {
				log.Printf("Error parsing date (%s): %v\n", row[4], err)
				http.Error(w, "Invalid date format", http.StatusBadRequest)
				return
			}

			_, err = tx.Exec(`
				INSERT INTO prices (id, name, category, price, create_date)
				VALUES ($1, $2, $3, $4, $5)
			`, id, itemName, category, price, createDate)
			if err != nil {
				log.Printf("Error inserting data into prices table: %v\n", err)
				http.Error(w, "Failed to insert data", http.StatusInternalServerError)
				return
			}
		}
	}

	if err = tx.Commit(); err != nil {
		log.Printf("Error committing transaction: %v\n", err)
		http.Error(w, "Failed to commit transaction", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Successfully inserted!"))
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
