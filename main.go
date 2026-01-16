package main

import (
	_ "embed"
	"log"
	"net/http"

	_ "modernc.org/sqlite"

	"tuto.sqlc.dev/app/go/handler"
)

//go:embed resources/schema.sql
var ddl string

func main() {
	log.Println("started...")
	http.HandleFunc("/", handler.UploadHandler)
	http.HandleFunc("/view", handler.ViewHandler)
	http.HandleFunc("/download", handler.DownloadHandler)

	log.Println("Server starting on port 8080...")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
