package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"os"
	"time"
)

const baseURL = "https://www.vworkai.com"

type Product struct {
	ID   string `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

type User struct {
	ID       string `json:"id"`
	TenantID string `json:"tenant_id"`
}

func main() {
	if len(os.Args) < 4 {
		fmt.Println("Usage: go run main.go <admin_user> <admin_pass> <tenant_id>")
		os.Exit(1)
	}
	adminUser := os.Args[1]
	adminPass := os.Args[2]
	tenantID := os.Args[3]

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 30 * time.Second}

	// 1. Login as vworkadmin
	fmt.Println("=> Step 1: Logging in as vworkadmin...")
	loginBody, _ := json.Marshal(map[string]string{"username": adminUser, "password": adminPass})
	loginReq, _ := http.NewRequest("POST", baseURL+"/vworkadmin/login", bytes.NewBuffer(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginResp, err := client.Do(loginReq)
	if err != nil {
		log.Fatalf("Admin login request failed: %v", err)
	}
	defer loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusOK {
		log.Fatalf("Admin login failed with status %d", loginResp.StatusCode)
	}
	fmt.Println("=> Login successful.")

	// 2. Get user_id from overview
	fmt.Println("=> Step 2: Finding a user_id for the tenant...")
	overviewReq, _ := http.NewRequest("GET", baseURL+"/api/v1/vworkadmin/overview", nil)
	overviewResp, err := client.Do(overviewReq)
	if err != nil {
		log.Fatalf("Failed to get overview data: %v", err)
	}
	defer overviewResp.Body.Close()
	var overviewData struct {
		Users []User `json:"users"`
	}
	if err := json.NewDecoder(overviewResp.Body).Decode(&overviewData); err != nil {
		log.Fatalf("Failed to parse overview JSON: %v", err)
	}
	var userID string
	for _, u := range overviewData.Users {
		if u.TenantID == tenantID {
			userID = u.ID
			break
		}
	}
	if userID == "" {
		log.Fatalf("Could not find any user for tenant_id: %s", tenantID)
	}
	fmt.Printf("=> Found user_id: %s\n", userID)

	// 3. Get impersonation JWT token
	fmt.Println("=> Step 3: Getting impersonation JWT token...")
	loginAsBody, _ := json.Marshal(map[string]string{"user_id": userID, "tenant_id": tenantID})
	loginAsReq, _ := http.NewRequest("POST", baseURL+"/api/v1/vworkadmin/login-as-user", bytes.NewBuffer(loginAsBody))
	loginAsReq.Header.Set("Content-Type", "application/json")
	loginAsResp, err := client.Do(loginAsReq)
	if err != nil {
		log.Fatalf("Login-as-user request failed: %v", err)
	}
	defer loginAsResp.Body.Close()
	var loginAsData struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(loginAsResp.Body).Decode(&loginAsData); err != nil || loginAsData.Token == "" {
		log.Fatalf("Failed to parse JWT from login-as-user response: %v", err)
	}
	jwtToken := loginAsData.Token
	fmt.Println("=> Successfully obtained JWT.")

	// 4. Get all products for the tenant and verify
	fmt.Println("\n=> VERIFICATION: Fetching product list to check for empty codes...")
	getProductsReq, _ := http.NewRequest("GET", baseURL+"/api/v1/products?limit=2000", nil)
	getProductsReq.Header.Set("Authorization", "Bearer "+jwtToken)
	getProductsResp, err := client.Do(getProductsReq)
	if err != nil {
		log.Fatalf("Failed to get products: %v", err)
	}
	defer getProductsResp.Body.Close()

	if getProductsResp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(getProductsResp.Body)
		log.Fatalf("Get products failed with status %d: %s", getProductsResp.StatusCode, string(bodyBytes))
	}

	var productsData struct {
		Data []Product `json:"data"`
	}
	if err := json.NewDecoder(getProductsResp.Body).Decode(&productsData); err != nil {
		log.Fatalf("Failed to parse products JSON: %v", err)
	}

	productsWithoutCode := []Product{}
	for _, p := range productsData.Data {
		if p.Code == "" {
			productsWithoutCode = append(productsWithoutCode, p)
		}
	}

	if len(productsWithoutCode) == 0 {
		fmt.Println("\nSUCCESS: All products seem to have a code.")
	} else {
		fmt.Printf("\nFAILURE: Found %d products that still have no code:\n", len(productsWithoutCode))
		for _, p := range productsWithoutCode {
			fmt.Printf("  - ID: %s, Name: %s\n", p.ID, p.Name)
		}
	}
}
