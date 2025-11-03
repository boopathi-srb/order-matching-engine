package main

import (
	"log"

	// Correctly import your two local packages
	"order-matching-engine/src/api"
	"order-matching-engine/src/engine"
)

func main() {
	log.Println("Initializing the matching engine...")
	eng := engine.NewMatchingEngine()
	srv := api.NewServer(eng)
	log.Println("Starting API server on :8080")
	if err := srv.Start(":8080"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}