package storage

import (
	"database/sql"
	"log"

	_ "github.com/lib/pq"
)

var db *sql.DB

// InitDB initializes the database connection
func InitDB() error {
	var err error
	dsn := "user=validator password=val1dat0r dbname=project-sem-1 host=localhost port=5432 sslmode=disable"
	db, err = sql.Open("postgres", dsn)
	if err != nil {
		return err
	}

	if err = db.Ping(); err != nil {
		return err
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS prices (
		id SERIAL PRIMARY KEY,
		category TEXT,
		item_name TEXT,
		price REAL
	)`)
	if err != nil {
		return err
	}

	log.Println("Database connected and table ensured.")
	return nil
}

// CloseDB closes the database connection
func CloseDB() {
	if db != nil {
		db.Close()
		log.Println("Database connection closed.")
	}
}

// GetDB provides access to the database instance
func GetDB() *sql.DB {
	return db
}
