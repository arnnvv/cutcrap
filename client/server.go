package main

import (
	"log"
	"net/http"
)

func main() {
	// Create a file server that serves files from the current directory
	fileServer := http.FileServer(http.Dir("./"))

	// Handle all requests by serving a file of the same name
	http.Handle("/", fileServer)

	// Start the server on port 8000
	log.Printf("Starting server on http://localhost:8000")
	err := http.ListenAndServe(":8000", nil)
	if err != nil {
		log.Fatal("Error starting server: ", err)
	}
}
