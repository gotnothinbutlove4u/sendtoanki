package main

import (
	"log"
	"net/http"

	_ "modernc.org/sqlite"

	"github.com/dawit909/sendtoanki/src/handler"
)

func main() {
	log.Println("started...")

	// Simple CORS middleware
	enableCORS := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// In production, replace '*' with your frontend's actual origin
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")

			// Handle preflight requests
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			next(w, r)
		}
	}

	// Register API Handlers
	http.HandleFunc("/api/upload", enableCORS(handler.UploadAPIHandler))
	http.HandleFunc("/api/download", enableCORS(handler.DownloadAPIHandler))
	// We use a prefix handler for delete to capture the word stem from the URL
	http.HandleFunc("/api/words/", enableCORS(handler.DeleteAPIHandler))

	log.Println("Server starting on port 8080...")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
