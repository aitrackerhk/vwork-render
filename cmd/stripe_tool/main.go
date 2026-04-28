// Stripe Product/Price Generator Tool
// Usage:
//   go run cmd/stripe_tool/main.go create-products   # Create products & prices from catalog JSON
//   go run cmd/stripe_tool/main.go list-products     # List existing products
//   go run cmd/stripe_tool/main.go sync              # Sync catalog JSON with Stripe price IDs
//
// Requires STRIPE_SECRET_KEY in .env or environment

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/stripe/stripe-go/v80"
	"github.com/stripe/stripe-go/v80/price"
	"github.com/stripe/stripe-go/v80/product"
)

type CatalogItem struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	NameEN      string            `json:"name_en,omitempty"`
	Description string            `json:"description,omitempty"`
	Group       string            `json:"group"`
	PriceID     string            `json:"price_id"`
	UnitAmount  int64             `json:"unit_amount"`
	Currency    string            `json:"currency"`
	ImageURL    string            `json:"image_url,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

const catalogPath = "config/hardware_purchase_catalog.json"

func main() {
	// Load .env
	_ = godotenv.Load()

	key := os.Getenv("STRIPE_SECRET_KEY")
	if key == "" {
		fmt.Println("❌ STRIPE_SECRET_KEY not set in .env or environment")
		os.Exit(1)
	}
	stripe.Key = key

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	switch cmd {
	case "create-products":
		createProducts()
	case "list-products":
		listProducts()
	case "sync":
		syncCatalog()
	case "help":
		printUsage()
	default:
		fmt.Printf("❌ Unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`
Stripe Product/Price Tool for Hardware Catalog

Usage:
  go run cmd/stripe_tool/main.go <command>

Commands:
  create-products   Create Stripe products & prices from catalog JSON
                    (skips items that already have price_id)
  list-products     List all Stripe products with "vwork_hardware" metadata
  sync              Read catalog, create missing products, update JSON with price IDs

Environment:
  STRIPE_SECRET_KEY   Required. Your Stripe secret key (sk_test_... or sk_live_...)

Catalog File:
  config/hardware_purchase_catalog.json

Language Strategy:
  - Stripe product name: Uses "name" field (Chinese) - customers see this at checkout
  - "name_en" is stored in metadata for internal reference
`)
}

func loadCatalog() ([]CatalogItem, error) {
	b, err := os.ReadFile(catalogPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read catalog: %w", err)
	}
	var items []CatalogItem
	if err := json.Unmarshal(b, &items); err != nil {
		return nil, fmt.Errorf("failed to parse catalog: %w", err)
	}
	return items, nil
}

func saveCatalog(items []CatalogItem) error {
	b, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal catalog: %w", err)
	}
	if err := os.WriteFile(catalogPath, b, 0644); err != nil {
		return fmt.Errorf("failed to write catalog: %w", err)
	}
	return nil
}

func createProducts() {
	items, err := loadCatalog()
	if err != nil {
		fmt.Printf("❌ %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("📦 Found %d items in catalog\n\n", len(items))

	for i, item := range items {
		if item.PriceID != "" {
			fmt.Printf("⏭️  [%s] Already has price_id: %s\n", item.ID, item.PriceID)
			continue
		}

		if item.UnitAmount <= 0 {
			fmt.Printf("⚠️  [%s] Skipped: unit_amount is 0 (set a price first)\n", item.ID)
			continue
		}

		// Use Chinese name for Stripe - customers see this at checkout
		stripeName := item.Name

		// Create Product
		prodParams := &stripe.ProductParams{
			Name:        stripe.String(stripeName),
			Description: stripe.String(item.Description),
			Metadata: map[string]string{
				"vwork_hardware": "true",
				"catalog_id":     item.ID,
				"group":          item.Group,
				"name_en":        item.NameEN,
			},
		}
		prod, err := product.New(prodParams)
		if err != nil {
			fmt.Printf("❌ [%s] Failed to create product: %v\n", item.ID, err)
			continue
		}
		fmt.Printf("✅ [%s] Created product: %s\n", item.ID, prod.ID)

		// Create Price
		currency := strings.ToLower(item.Currency)
		if currency == "" {
			currency = "hkd"
		}
		priceParams := &stripe.PriceParams{
			Product:    stripe.String(prod.ID),
			Currency:   stripe.String(currency),
			UnitAmount: stripe.Int64(item.UnitAmount), // in cents
			Metadata: map[string]string{
				"catalog_id": item.ID,
			},
		}
		pr, err := price.New(priceParams)
		if err != nil {
			fmt.Printf("❌ [%s] Failed to create price: %v\n", item.ID, err)
			continue
		}
		fmt.Printf("✅ [%s] Created price: %s (%d %s)\n", item.ID, pr.ID, item.UnitAmount, currency)

		items[i].PriceID = pr.ID
	}

	// Save updated catalog
	if err := saveCatalog(items); err != nil {
		fmt.Printf("❌ Failed to save catalog: %v\n", err)
		os.Exit(1)
	}
	fmt.Println()
	fmt.Println("✅ Catalog saved with updated price_ids")
}

func listProducts() {
	fmt.Println("📦 Listing Stripe products with vwork_hardware metadata...")

	params := &stripe.ProductListParams{}
	params.Limit = stripe.Int64(100)

	iter := product.List(params)
	count := 0
	for iter.Next() {
		p := iter.Product()
		if p.Metadata["vwork_hardware"] == "true" {
			count++
			fmt.Printf("[%s] %s\n", p.Metadata["catalog_id"], p.Name)
			fmt.Printf("    Product ID: %s\n", p.ID)
			fmt.Printf("    Group: %s\n", p.Metadata["group"])
			fmt.Printf("    Name (ZH): %s\n", p.Metadata["name_zh"])

			// List prices for this product
			priceParams := &stripe.PriceListParams{
				Product: stripe.String(p.ID),
			}
			priceIter := price.List(priceParams)
			for priceIter.Next() {
				pr := priceIter.Price()
				fmt.Printf("    Price: %s (%d %s)\n", pr.ID, pr.UnitAmount, strings.ToUpper(string(pr.Currency)))
			}
			fmt.Println()
		}
	}
	if err := iter.Err(); err != nil {
		fmt.Printf("❌ Error listing products: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found %d vwork_hardware products\n", count)
}

func syncCatalog() {
	items, err := loadCatalog()
	if err != nil {
		fmt.Printf("❌ %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("🔄 Syncing %d catalog items...\n\n", len(items))

	// Build a map of existing products by catalog_id
	existingProducts := make(map[string]*stripe.Product)
	existingPrices := make(map[string]*stripe.Price)

	params := &stripe.ProductListParams{}
	params.Limit = stripe.Int64(100)
	iter := product.List(params)
	for iter.Next() {
		p := iter.Product()
		if p.Metadata["vwork_hardware"] == "true" && p.Metadata["catalog_id"] != "" {
			existingProducts[p.Metadata["catalog_id"]] = p

			// Get first price
			priceParams := &stripe.PriceListParams{
				Product: stripe.String(p.ID),
			}
			priceParams.Limit = stripe.Int64(1)
			priceIter := price.List(priceParams)
			if priceIter.Next() {
				existingPrices[p.Metadata["catalog_id"]] = priceIter.Price()
			}
		}
	}

	updated := 0
	for i, item := range items {
		if item.PriceID != "" {
			fmt.Printf("✓ [%s] Already synced: %s\n", item.ID, item.PriceID)
			continue
		}

		// Check if product exists in Stripe
		if pr, ok := existingPrices[item.ID]; ok {
			items[i].PriceID = pr.ID
			fmt.Printf("🔗 [%s] Found existing price: %s\n", item.ID, pr.ID)
			updated++
			continue
		}

		// Need to create
		if item.UnitAmount <= 0 {
			fmt.Printf("⚠️  [%s] Skipped: set unit_amount first\n", item.ID)
			continue
		}

		// Use Chinese name - customers see this at checkout
		stripeName := item.Name

		prodParams := &stripe.ProductParams{
			Name:        stripe.String(stripeName),
			Description: stripe.String(item.Description),
			Metadata: map[string]string{
				"vwork_hardware": "true",
				"catalog_id":     item.ID,
				"group":          item.Group,
				"name_en":        item.NameEN,
			},
		}
		prod, err := product.New(prodParams)
		if err != nil {
			fmt.Printf("❌ [%s] Failed to create product: %v\n", item.ID, err)
			continue
		}

		currency := strings.ToLower(item.Currency)
		if currency == "" {
			currency = "hkd"
		}
		priceParams := &stripe.PriceParams{
			Product:    stripe.String(prod.ID),
			Currency:   stripe.String(currency),
			UnitAmount: stripe.Int64(item.UnitAmount),
		}
		pr, err := price.New(priceParams)
		if err != nil {
			fmt.Printf("❌ [%s] Failed to create price: %v\n", item.ID, err)
			continue
		}

		items[i].PriceID = pr.ID
		fmt.Printf("✅ [%s] Created: %s\n", item.ID, pr.ID)
		updated++
	}

	if updated > 0 {
		if err := saveCatalog(items); err != nil {
			fmt.Printf("❌ Failed to save catalog: %v\n", err)
			os.Exit(1)
		}
		fmt.Println()
		fmt.Printf("✅ Updated %d items in catalog\n", updated)
	} else {
		fmt.Println()
		fmt.Println("✓ No updates needed")
	}
}
