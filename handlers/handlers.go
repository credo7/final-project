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

	totalItems := 0
	totalCategoriesMap := make(map[string]bool)
	totalPrice := 0.0

	for _, zipFile := range zipReader.File {
		fileReader, err := zipFile.Open()
		if err != nil {
			log.Printf("Error reading file in ZIP: %v\n", err)
			http.Error(w, "Failed to read file in zip", http.StatusInternalServerError)
			return
		}
		defer fileReader.Close()

		csvReader := csv.NewReader(fileReader)

		_, err = csvReader.Read()
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

			// row format: [id, name, category, price, create_date]
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

			createDate := row[4] // If needed, parse as time.Time

			_, err = tx.Exec(`
                INSERT INTO prices (id, name, category, price, create_date)
                VALUES ($1, $2, $3, $4, $5)
            `, id, itemName, category, price, createDate)
			if err != nil {
				log.Printf("Error inserting data into prices table: %v\n", err)
				tx.Rollback()
				http.Error(w, "Failed to insert data", http.StatusInternalServerError)
				return
			}

			totalItems++
			totalCategoriesMap[category] = true
			totalPrice += price
		}
	}

	if err = tx.Commit(); err != nil {
		log.Printf("Error committing transaction: %v\n", err)
		http.Error(w, "Failed to commit transaction", http.StatusInternalServerError)
		return
	}

	totalCategories := len(totalCategoriesMap)
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

	// Буфер в памяти, куда запишем CSV
	var csvBuffer bytes.Buffer
	csvWriter := csv.NewWriter(&csvBuffer)

	// Заполняем CSV
	for rows.Next() {
		var (
			id         int
			name       string
			category   string
			price      float64
			createDate string // либо time.Time, если нужно парсить
		)

		if err := rows.Scan(&id, &name, &category, &price, &createDate); err != nil {
			log.Printf("Failed to read data from DB: %v", err)
			http.Error(w, "Failed to read data from DB", http.StatusInternalServerError)
			return
		}

		// Преобразуем price во float -> string
		priceStr := strconv.FormatFloat(price, 'f', 2, 64)

		record := []string{
			strconv.Itoa(id),
			name,
			category,
			priceStr,
			createDate,
		}
		if err := csvWriter.Write(record); err != nil {
			log.Printf("Failed to write record to CSV: %v", err)
			http.Error(w, "Failed to write CSV", http.StatusInternalServerError)
			return
		}
	}
	csvWriter.Flush() // сбрасываем буфер в csvBuffer

	// Если в rows была ошибка после Next()
	if err := rows.Err(); err != nil {
		log.Printf("Error during rows iteration: %v", err)
		http.Error(w, "Failed to iterate rows", http.StatusInternalServerError)
		return
	}

	// Готовим заголовки ответа
	w.Header().Set("Content-Disposition", "attachment; filename=response.zip")
	w.Header().Set("Content-Type", "application/zip")

	// Создаём zip в потоке ответа
	zipWriter := zip.NewWriter(w)
	defer zipWriter.Close()

	// Добавляем файл data.csv внутрь архива
	fileWriter, err := zipWriter.Create("data.csv")
	if err != nil {
		log.Printf("Failed to create file in zip: %v", err)
		http.Error(w, "Failed to create file in zip", http.StatusInternalServerError)
		return
	}

	// Пишем CSV-данные в data.csv внутри архива
	if _, err := io.Copy(fileWriter, &csvBuffer); err != nil {
		log.Printf("Failed to copy data to zip: %v", err)
		http.Error(w, "Failed to copy data to zip", http.StatusInternalServerError)
		return
	}
}
