package handlers

import (
	"fmt"
	"image"
	"image/jpeg"
	"nwork/internal/middleware"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"golang.org/x/image/draw"
)

// CropImage 處理圖片裁剪
func CropImage(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	// 獲取上傳的文件
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "No file uploaded"})
	}

	// 獲取裁剪參數
	xStr := c.FormValue("x")
	yStr := c.FormValue("y")
	widthStr := c.FormValue("width")
	heightStr := c.FormValue("height")
	outputWidthStr := c.FormValue("outputWidth")
	outputHeightStr := c.FormValue("outputHeight")

	if xStr == "" || yStr == "" || widthStr == "" || heightStr == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Missing crop parameters"})
	}

	// 解析裁剪參數
	cropX, err := strconv.ParseFloat(xStr, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("Invalid x parameter: %s", xStr)})
	}
	cropY, err := strconv.ParseFloat(yStr, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("Invalid y parameter: %s", yStr)})
	}
	cropWidth, err := strconv.ParseFloat(widthStr, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("Invalid crop width: %s", widthStr)})
	}
	cropHeight, err := strconv.ParseFloat(heightStr, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("Invalid crop height: %s", heightStr)})
	}
	
	// 驗證裁剪尺寸是否有效
	if cropWidth <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("Invalid crop width: must be greater than 0, got %f", cropWidth)})
	}
	if cropHeight <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("Invalid crop height: must be greater than 0, got %f", cropHeight)})
	}

	// 解析輸出尺寸（可選，如果沒有則使用裁剪尺寸）
	outputWidth := int(cropWidth)
	outputHeight := int(cropHeight)
	if outputWidthStr != "" {
		if w, err := strconv.Atoi(outputWidthStr); err == nil && w > 0 {
			outputWidth = w
		}
	}
	if outputHeightStr != "" {
		if h, err := strconv.Atoi(outputHeightStr); err == nil && h > 0 {
			outputHeight = h
		}
	}

	// 檢查文件大小
	maxFileSize := int64(10 * 1024 * 1024) // 默認 10MB
	if uploadConfig != nil && uploadConfig.MaxFileSize > 0 {
		maxFileSize = int64(uploadConfig.MaxFileSize)
	}
	if file.Size > maxFileSize {
		return c.Status(400).JSON(fiber.Map{
			"error": fmt.Sprintf("File size exceeds maximum allowed size of %d MB", maxFileSize/(1024*1024)),
		})
	}

	// 打開文件
	src, err := file.Open()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to open uploaded file"})
	}
	defer src.Close()

	// 解碼圖片
	img, format, err := image.Decode(src)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Failed to decode image: " + err.Error()})
	}

	// 獲取圖片邊界
	bounds := img.Bounds()
	imgWidth := bounds.Dx()
	imgHeight := bounds.Dy()

	// 將浮點數裁剪坐標轉換為整數（四捨五入）
	cropXInt := int(cropX + 0.5)
	cropYInt := int(cropY + 0.5)
	cropWidthInt := int(cropWidth + 0.5)
	cropHeightInt := int(cropHeight + 0.5)

	// 驗證裁剪區域是否在圖片範圍內
	if cropXInt < 0 || cropYInt < 0 || cropXInt+cropWidthInt > imgWidth || cropYInt+cropHeightInt > imgHeight {
		return c.Status(400).JSON(fiber.Map{
			"error": fmt.Sprintf("Crop area out of bounds: image size is %dx%d, crop area is (%d,%d) %dx%d",
				imgWidth, imgHeight, cropXInt, cropYInt, cropWidthInt, cropHeightInt),
		})
	}

	// 創建裁剪區域
	cropRect := image.Rect(cropXInt, cropYInt, cropXInt+cropWidthInt, cropYInt+cropHeightInt)

	// 裁剪圖片
	croppedImg := image.NewRGBA(cropRect)
	draw.Draw(croppedImg, croppedImg.Bounds(), img, cropRect.Min, draw.Src)

	// 如果需要調整輸出尺寸，進行縮放
	var finalImg image.Image = croppedImg
	if outputWidth != cropWidthInt || outputHeight != cropHeightInt {
		resized := image.NewRGBA(image.Rect(0, 0, outputWidth, outputHeight))
		draw.CatmullRom.Scale(resized, resized.Bounds(), croppedImg, croppedImg.Bounds(), draw.Over, nil)
		finalImg = resized
	}

	// 創建上傳目錄
	uploadDir := filepath.Join("web", "uploads", tenantID.String(), time.Now().Format("2006-01-02"))
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create upload directory"})
	}

	// 生成唯一文件名
	filename := fmt.Sprintf("%s.jpg", uuid.New().String())
	filePath := filepath.Join(uploadDir, filename)

	// 保存裁剪後的圖片
	dst, err := os.Create(filePath)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create file"})
	}
	defer dst.Close()

	// 根據原始格式選擇編碼方式（但統一保存為 JPEG）
	// 如果原始是 PNG 且需要透明背景，可以考慮保存為 PNG，但這裡統一用 JPEG
	imgFormat := strings.ToLower(format)
	if imgFormat == "png" {
		// PNG 轉 JPEG（可能會丟失透明度，但通常裁剪不需要透明度）
		if err := jpeg.Encode(dst, finalImg, &jpeg.Options{Quality: 90}); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to encode JPEG image: " + err.Error()})
		}
	} else {
		// JPEG 或其他格式
		if err := jpeg.Encode(dst, finalImg, &jpeg.Options{Quality: 90}); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to encode JPEG image: " + err.Error()})
		}
	}

	// 返回文件 URL
	fileURL := fmt.Sprintf("/uploads/%s/%s/%s", tenantID.String(), time.Now().Format("2006-01-02"), filename)

	return c.JSON(fiber.Map{
		"url":    fileURL,
		"path":   filePath,
		"width":  outputWidth,
		"height": outputHeight,
	})
}

