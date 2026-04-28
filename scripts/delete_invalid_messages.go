package main

import (
	"fmt"
	"log"
	"nwork/config"
	"nwork/internal/database"

	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️  Warning: .env file not found, using environment variables")
	}

	cfg := config.Load()
	if err := database.Connect(cfg); err != nil {
		log.Fatalf("connect db failed: %v", err)
	}
	defer database.Close()

	result := database.DB.Exec("DELETE FROM messages WHERE (message_type IS DISTINCT FROM 'ai_chat') AND to_user_id IS NULL AND to_customer_id IS NULL")
	if result.Error != nil {
		log.Fatalf("delete failed: %v", result.Error)
	}
	fmt.Printf("Deleted rows: %d\n", result.RowsAffected)
}
