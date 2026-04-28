package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const (
	apiKey = "AIzaSyCHTCHR_Mod0J1zZSj5MHxKAZtIJXHlEC4"
	model  = "veo-3.1-generate-preview"
	base   = "https://generativelanguage.googleapis.com/v1beta"
)

func main() {
	// Step 1: Read a real test image from disk
	imgPath := `C:\Users\tednv\vsys\vwork\web\static\vaiicon.png`
	imgBytes, err := os.ReadFile(imgPath)
	if err != nil {
		fmt.Printf("FATAL: cannot read image %s: %v\n", imgPath, err)
		os.Exit(1)
	}
	b64Img := base64.StdEncoding.EncodeToString(imgBytes)
	fmt.Printf("[1] Test image: %s (%d bytes, base64: %d chars)\n", imgPath, len(imgBytes), len(b64Img))

	// Step 2: Submit predictLongRunning with reference image
	fmt.Println("\n[2] Submitting predictLongRunning...")
	opName, err := submitGenerate(b64Img)
	if err != nil {
		fmt.Printf("FATAL: submit failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[2] Operation submitted: %s\n", opName)

	// Step 3: Poll using GET /v1beta/{operationName} (correct Gemini API way)
	fmt.Println("\n[3] Polling with GET (correct Gemini API method)...")
	err = pollWithGET(opName)
	if err != nil {
		fmt.Printf("[3] GET poll result: %v\n", err)
	}

	// Step 4: Also try POST fetchPredictOperation (to confirm it fails)
	fmt.Println("\n[4] Polling with POST fetchPredictOperation (expected to fail)...")
	err = pollWithFetchPredict(opName)
	if err != nil {
		fmt.Printf("[4] fetchPredictOperation result: %v\n", err)
	}
}

func submitGenerate(b64Img string) (string, error) {
	url := fmt.Sprintf("%s/models/%s:predictLongRunning", base, model)

	body := map[string]interface{}{
		"instances": []map[string]interface{}{
			{
				"prompt": "A person walking in a park on a sunny day",
				"referenceImages": []map[string]interface{}{
					{
						"referenceType": "asset",
						"image": map[string]interface{}{
							"bytesBase64Encoded": b64Img,
							"mimeType":           "image/png",
						},
					},
				},
			},
		},
		"parameters": map[string]interface{}{
			"aspectRatio":     "16:9",
			"durationSeconds": 8,
			"resolution":      "720p",
			"sampleCount":     1,
		},
	}

	payload, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", apiKey)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP error: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	fmt.Printf("    Submit response status: %d\n", resp.StatusCode)
	fmt.Printf("    Submit response body: %s\n", truncate(string(respBody), 500))

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse error: %v", err)
	}
	return result.Name, nil
}

func pollWithGET(opName string) error {
	// Gemini API: GET /v1beta/{operationName}
	url := fmt.Sprintf("%s/%s", base, opName)

	for i := 0; i < 60; i++ {
		fmt.Printf("    [GET poll #%d] %s\n", i+1, url)

		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("x-goog-api-key", apiKey)

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("HTTP error: %v", err)
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		fmt.Printf("    Status: %d, Body: %s\n", resp.StatusCode, truncate(string(respBody), 300))

		if resp.StatusCode != 200 {
			return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
		}

		var result struct {
			Done     bool            `json:"done"`
			Error    json.RawMessage `json:"error"`
			Response json.RawMessage `json:"response"`
		}
		json.Unmarshal(respBody, &result)

		if result.Done {
			if result.Error != nil {
				fmt.Printf("    DONE with error: %s\n", string(result.Error))
			} else {
				fmt.Printf("    DONE! Response: %s\n", truncate(string(result.Response), 500))
			}
			return nil
		}

		fmt.Println("    Not done yet, waiting 10s...")
		time.Sleep(10 * time.Second)
	}
	return fmt.Errorf("timeout after 60 polls")
}

func pollWithFetchPredict(opName string) error {
	url := fmt.Sprintf("%s/models/%s:fetchPredictOperation", base, model)
	body := map[string]interface{}{
		"operationName": opName,
	}
	payload, _ := json.Marshal(body)

	fmt.Printf("    [POST fetchPredictOperation] %s\n", url)
	fmt.Printf("    Body: %s\n", string(payload))

	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP error: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	fmt.Printf("    Status: %d, Body: %s\n", resp.StatusCode, truncate(string(respBody), 300))

	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}
