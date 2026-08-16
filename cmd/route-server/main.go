package main

import (
	"log"
	// "os"
	"net/http"
	"route-optimizer-go/internal/httpapi"
)

func main() {
	server, err := httpapi.NewServer()
	if err != nil {
		log.Fatal("failed to construct server: %v", err)
	}
	log.Println("server is listening on :8080")
	if err := http.ListenAndServe(":8080", server); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}