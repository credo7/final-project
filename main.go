package main

import (
	"fmt"
	"github.com/go-chi/chi/v5"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
	"project_sem/handlers"
	"project_sem/storage"
)

func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				fmt.Printf("Panic: %v\n", err)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func main() {
	if err := storage.InitDB(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer storage.CloseDB()

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	//r.Use(middleware.Recoverer)
	r.Use(recoveryMiddleware)

	r.Route("/api/v0/prices", func(r chi.Router) {
		r.Post("/", handlers.PostPricesHandler)
		r.Get("/", handlers.GetPricesHandler)
	})

	log.Println("Server is running on port 8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}
