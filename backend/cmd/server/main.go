package main

import (
	"fmt"
	"log"

	"auto_test/backend/internal/config"
	"auto_test/backend/internal/router"
	"auto_test/backend/internal/server"
)

func main() {
	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	fmt.Printf("Server starting on port %s\n", cfg.Server.Port)
	router := router.SetupRouter()
	if err := server.Run(router, cfg.Server.Port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
