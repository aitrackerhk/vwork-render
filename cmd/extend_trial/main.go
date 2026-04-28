package main

import (
	"fmt"
	"log"
	"nwork/config"
	"nwork/internal/database"
	"nwork/internal/models"
	"os"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: go run cmd/extend_trial/main.go <email> [years]")
	}
	email := os.Args[1]
	years := 1
	if len(os.Args) > 2 {
		// Optional: parse years, for now default to 1
		fmt.Println("Defaulting to 1 year extension.")
	}

	// Load .env
	if err := godotenv.Load(); err != nil {
		log.Println("Note: .env file not found, trying environment variables")
	}

	cfg := config.Load()
	if err := database.Connect(cfg); err != nil {
		log.Fatalf("failed to connect DB: %v", err)
	}

	var user models.User
	if err := database.DB.Where("email = ?", email).First(&user).Error; err != nil {
		log.Fatalf("User not found with email %s: %v", email, err)
	}

	if user.TenantID == nil {
		log.Fatal("User has no tenant associated")
	}

	var tenant models.Tenant
	if err := database.DB.First(&tenant, user.TenantID).Error; err != nil {
		log.Fatalf("Tenant not found: %v", err)
	}

	fmt.Printf("Current Tenant Status:\n")
	fmt.Printf("Plan: %s\n", tenant.Plan)
	fmt.Printf("Status: %s\n", tenant.Status)
	if tenant.TrialExpiresAt != nil {
		fmt.Printf("Trial Expires: %s\n", tenant.TrialExpiresAt.Format(time.RFC3339))
	} else {
		fmt.Printf("Trial Expires: nil\n")
	}

	// Extend for 1 year from NOW
	newExpiry := time.Now().AddDate(years, 0, 0)

	tenant.TrialExpiresAt = &newExpiry
	tenant.Plan = "trial"
	tenant.Status = "active"

	// Also clear SubscriptionID if we are forcing a trial, to avoid confusion?
	// Or leave it. Better leave it, but ensure Plan is 'trial'.

	if err := database.DB.Save(&tenant).Error; err != nil {
		log.Fatalf("Failed to update tenant: %v", err)
	}

	fmt.Println("--------------------------------------------------")
	fmt.Printf("Successfully extended trial for user: %s\n", email)
	fmt.Printf("Tenant: %s (%s)\n", tenant.Name, tenant.Subdomain)
	fmt.Printf("New Expiry Date: %s\n", newExpiry.Format("2006-01-02"))
}
