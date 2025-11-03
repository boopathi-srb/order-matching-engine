package main

import (
	"log"

	// Correctly import your two local packages
	"order-matching-engine/src/api"
	"order-matching-engine/src/engine"
)

func main() {
	// 1. Create the matching engine
	// This is the core logic, and it's thread-safe
	log.Println("Initializing the matching engine...")
	eng := engine.NewMatchingEngine()

	// 2. Create the API server and pass the engine to it
	// The server handles all HTTP requests and calls the engine
	srv := api.NewServer(eng)

	// 3. Start the server
	// We'll run it on port 8080
	log.Println("Starting API server on :8080")
	if err := srv.Start(":8080"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}