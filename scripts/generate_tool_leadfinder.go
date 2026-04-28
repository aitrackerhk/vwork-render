//go:build ignore
// +build ignore

// Script to generate a single tool card image for Lead Finder.
// Usage: go run scripts/generate_tool_leadfinder.go

package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	apiKey     = "AIzaSyCHTCHR_Mod0J1zZSj5MHxKAZtIJXHlEC4"
	model      = "gemini-3-pro-image-preview"
	outputPath = "web/static/tool_leadfinder.png"
	apiBaseURL = "https://generativelanguage.googleapis.com/v1beta/models"
)

const prompt = `Generate image with 16:9 aspect ratio: A clean, modern SaaS marketing illustration for an AI-powered automatic lead generation system. Show a stylized dashboard interface with a friendly robot/AI assistant icon in the center, scanning through business cards and company listings. Around it, show a visual pipeline: search icon -> filter -> qualified leads list. Use a professional blue-to-purple gradient color scheme. Flat design, minimalist style, no text on the image. Light clean background.`

type geminiRequest struct {
	Contents         []geminiContent `json:"contents"`
	GenerationConfig geminiGenConfig `json:"generationConfig"`
}
type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}
type geminiPart struct {
	Text string `json:"text,omitempty"`
}
type geminiGenConfig struct {
	ResponseModalities []string `json:"responseModalities"`
}
type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				InlineData *struct {
					MimeType string `json:"mimeType"`
					Data     string `json:"data"`
				} `json:"inlineData,omitempty"`
				Text string `json:"text,omitempty"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error,omitempty"`
}

func main() {
	fmt.Println("Generating Lead Finder tool card image...")

	reqBody := geminiRequest{
		Contents: []geminiContent{
			{Parts: []geminiPart{{Text: prompt}}},
		},
		GenerationConfig: geminiGenConfig{
			ResponseModalities: []string{"IMAGE", "TEXT"},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		fmt.Printf("ERROR marshal: %v\n", err)
		os.Exit(1)
	}

	url := fmt.Sprintf("%s/%s:generateContent?key=%s", apiBaseURL, model, apiKey)
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Post(url, "application/json", strings.NewReader(string(jsonData)))
	if err != nil {
		fmt.Printf("ERROR API call: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("ERROR read response: %v\n", err)
		os.Exit(1)
	}

	if resp.StatusCode != 200 {
		fmt.Printf("ERROR API %d: %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}

	var gemResp geminiResponse
	if err := json.Unmarshal(body, &gemResp); err != nil {
		fmt.Printf("ERROR parse: %v\n", err)
		os.Exit(1)
	}

	if gemResp.Error != nil {
		fmt.Printf("ERROR API: %s\n", gemResp.Error.Message)
		os.Exit(1)
	}

	for _, cand := range gemResp.Candidates {
		for _, part := range cand.Content.Parts {
			if part.InlineData != nil && part.InlineData.Data != "" {
				imgData, err := base64.StdEncoding.DecodeString(part.InlineData.Data)
				if err != nil {
					fmt.Printf("ERROR decode: %v\n", err)
					os.Exit(1)
				}
				if err := os.WriteFile(outputPath, imgData, 0644); err != nil {
					fmt.Printf("ERROR write: %v\n", err)
					os.Exit(1)
				}
				fmt.Printf("Saved %s (%d bytes)\n", outputPath, len(imgData))
				return
			}
		}
	}

	fmt.Println("ERROR: no image in response")
	fmt.Printf("Raw response: %s\n", string(body))
	os.Exit(1)
}
