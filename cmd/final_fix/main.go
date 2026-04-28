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

	// 1. Login
	fmt.Println("=> Step 1: Logging in as vworkadmin...")
	loginBody, _ := json.Marshal(map[string]string{"username": adminUser, "password": adminPass})
	loginReq, _ := http.NewRequest("POST", baseURL+"/vworkadmin/login", bytes.NewBuffer(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginResp, err := client.Do(loginReq)
	if err != nil || loginResp.StatusCode != http.StatusOK {
		log.Fatalf("Admin login failed.")
	}
	loginResp.Body.Close()
	fmt.Println("=> Login successful.")

	// 2. Get user_id
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
	json.NewDecoder(overviewResp.Body).Decode(&overviewData)
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

	// 3. Get JWT token
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
	json.NewDecoder(loginAsResp.Body).Decode(&loginAsData)
	jwtToken := loginAsData.Token
	if jwtToken == "" {
		log.Fatal("Failed to get JWT token.")
	}
	fmt.Println("=> Successfully obtained JWT.")

	// 4. Get products
	fmt.Println("=> Step 4: Fetching product list...")
	getProductsReq, _ := http.NewRequest("GET", baseURL+"/api/v1/products?limit=2000", nil)
	getProductsReq.Header.Set("Authorization", "Bearer "+jwtToken)
	getProductsResp, err := client.Do(getProductsReq)
	if err != nil {
		log.Fatalf("Failed to get products: %v", err)
	}
	defer getProductsResp.Body.Close()
	var productsData struct {
		Data []Product `json:"data"`
	}
	json.NewDecoder(getProductsResp.Body).Decode(&productsData)

	productsToFix := []Product{}
	for _, p := range productsData.Data {
		if p.Code == "" {
			productsToFix = append(productsToFix, p)
		}
	}
	fmt.Printf("=> Found %d products to fix.\n", len(productsToFix))

	// 5. Update products with generated codes
	if len(productsToFix) == 0 {
		fmt.Println("=> No products needed fixing. Done.")
		return
	}

	fmt.Println("=> Step 5: Updating products with locally generated codes...")
	updateCount := 0
	for i, p := range productsToFix {
		// Generate a simple, unique code locally
		newCode := fmt.Sprintf("P-%04d", i+1)

		updateBody, _ := json.Marshal(map[string]string{
			"code": newCode,
		})
		updateReq, _ := http.NewRequest("PUT", baseURL+"/api/v1/products/"+p.ID, bytes.NewBuffer(updateBody))
		updateReq.Header.Set("Authorization", "Bearer "+jwtToken)
		updateReq.Header.Set("Content-Type", "application/json")

		updateResp, err := client.Do(updateReq)
		if err != nil {
			fmt.Printf("  - FAILED to update product %s (%s): %v\n", p.Name, p.ID, err)
			continue
		}

		if updateResp.StatusCode == http.StatusOK {
			updateCount++
			fmt.Printf("  - SUCCESS: Updated product %s (%s) with code %s\n", p.Name, p.ID, newCode)
		} else {
			bodyBytes, _ := io.ReadAll(updateResp.Body)
			fmt.Printf("  - FAILED to update product %s (%s) with status %d: %s\n", p.Name, p.ID, updateResp.StatusCode, string(bodyBytes))
		}
		updateResp.Body.Close()
		time.Sleep(150 * time.Millisecond) // Be respectful to the server
	}

	fmt.Printf("\n=> All done. Attempted to update %d products, %d were successful.\n", len(productsToFix), updateCount)

	// Final verification step
	fmt.Println("\n=> Final Verification: Re-fetching products to confirm changes...")
	verifyReq, _ := http.NewRequest("GET", baseURL+"/api/v1/products?limit=2000", nil)
	verifyReq.Header.Set("Authorization", "Bearer "+jwtToken)
	verifyResp, err := client.Do(verifyReq)
	if err != nil {
		log.Fatalf("Verification failed: %v", err)
	}
	defer verifyResp.Body.Close()

	var verifyData struct {
		Data []Product `json:"data"`
	}
	json.NewDecoder(verifyResp.Body).Decode(&verifyData)

	remainingIssues := 0
	for _, p := range verifyData.Data {
		if p.Code == "" {
			remainingIssues++
			fmt.Printf("  - VERIFICATION FAILED: Product %s (ID: %s) still has no code.\n", p.Name, p.ID)
		}
	}

	if remainingIssues == 0 {
		fmt.Println("=> VERIFICATION PASSED: All products now have a code.")
	} else {
		fmt.Printf("=> VERIFICATION FAILED: %d products still have issues.\n", remainingIssues)
	}
}
