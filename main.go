package main

import (
	"fmt"
	"net/http"

	"order-matching-engine/src/api"
	"order-matching-engine/src/engine"
)

func main() {
	// 1. Initialize the Matching Engine
	me := engine.NewMatchingEngine()

	// 2. Create the API Server
	server := api.NewServer(me)

	// 3. Start the HTTP server
	port := 8080
	fmt.Printf("Starting server on :%d\n", port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), server); err != nil {
		fmt.Printf("Server failed to start: %v\n", err)
	}
}
