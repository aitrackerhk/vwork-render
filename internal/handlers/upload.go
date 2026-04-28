package handlers

import (
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"nwork/config"
	"nwork/internal/middleware"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"golang.org/x/image/draw"
)

// uploadConfig 存儲上傳配置（在 main.go 中初始化）
var uploadConfig *config.UploadConfig

// SetUploadConfig 設置上傳配置（由 main.go 調用）
func SetUploadConfig(cfg *config.UploadConfig) {
	uploadConfig = cfg
}

func saveUploadedImageForTenant(tenantID uuid.UUID, file *multipart.FileHeader) (fileURL string, filePath string, width int, height int, resized bool, err error) {
	if tenantID == uuid.Nil {
		return "", "", 0, 0, false, fmt.Errorf("tenant_id is required")
	}
	if file == nil {
		return "", "", 0, 0, false, fmt.Errorf("file is required")
	}

	// 檢查文件大小
	maxFileSize := int64(10 * 1024 * 1024) // 默認 10MB
	if uploadConfig != nil && uploadConfig.MaxFileSize > 0 {
		maxFileSize = int64(uploadConfig.MaxFileSize)
	}
	if file.Size > maxFileSize {
		return "", "", 0, 0, false, fmt.Errorf("File size exceeds maximum allowed size of %d MB", maxFileSize/(1024*1024))
	}

	// 注意：Content-Type 可能不可靠（canvas / cropper 上傳時常見 application/octet-stream）
	fileHeader := file.Header.Get("Content-Type")

	// 創建上傳目錄（按租戶和日期組織）
	uploadDir := filepath.Join("web", "uploads", tenantID.String(), time.Now().Format("2006-01-02"))
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return "", "", 0, 0, false, fmt.Errorf("Failed to create upload directory")
	}

	// 生成唯一文件名
	ext := filepath.Ext(file.Filename)
	filename := fmt.Sprintf("%s%s", uuid.New().String(), ext)
	filePath = filepath.Join(uploadDir, filename)

	// 打開文件
	src, err := file.Open()
	if err != nil {
		return "", "", 0, 0, false, fmt.Errorf("Failed to open uploaded file")
	}
	defer src.Close()

	// 讀取圖片並檢查/調整分辨率
	var img image.Image
	var format string
	contentType := strings.ToLower(fileHeader)
	if strings.Contains(contentType, "gif") {
		gifImg, err := gif.Decode(src)
		if err != nil {
			return "", "", 0, 0, false, fmt.Errorf("Failed to decode GIF image: %s", err.Error())
		}
		img = gifImg
		format = "gif"
	} else {
		decodedImg, decodedFormat, err := image.Decode(src)
		if err != nil {
			return "", "", 0, 0, false, fmt.Errorf("Only image files are allowed")
		}
		img = decodedImg
		format = decodedFormat
	}

	maxResolution := 1000
	if uploadConfig != nil {
		maxResolution = uploadConfig.MaxResolution
	}

	bounds := img.Bounds()
	width = bounds.Dx()
	height = bounds.Dy()

	if width > maxResolution || height > maxResolution {
		var newWidth, newHeight int
		if width > height {
			newWidth = maxResolution
			newHeight = int(float64(height) * float64(maxResolution) / float64(width))
		} else {
			newHeight = maxResolution
			newWidth = int(float64(width) * float64(maxResolution) / float64(height))
		}
		resizedImg := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
		draw.CatmullRom.Scale(resizedImg, resizedImg.Bounds(), img, bounds, draw.Over, nil)
		img = resizedImg
		width = newWidth
		height = newHeight
		resized = true
	}

	// 保存調整後的圖片
	dst, err := os.Create(filePath)
	if err != nil {
		return "", "", 0, 0, false, fmt.Errorf("Failed to create file")
	}
	defer dst.Close()

	switch strings.ToLower(format) {
	case "jpeg", "jpg":
		if err := jpeg.Encode(dst, img, &jpeg.Options{Quality: 85}); err != nil {
			return "", "", 0, 0, false, fmt.Errorf("Failed to encode JPEG image: %s", err.Error())
		}
	case "png":
		encoder := &png.Encoder{CompressionLevel: png.BestCompression}
		if err := encoder.Encode(dst, img); err != nil {
			return "", "", 0, 0, false, fmt.Errorf("Failed to encode PNG image: %s", err.Error())
		}
	case "gif":
		if err := jpeg.Encode(dst, img, &jpeg.Options{Quality: 85}); err != nil {
			return "", "", 0, 0, false, fmt.Errorf("Failed to encode GIF as JPEG: %s", err.Error())
		}
		oldPath := filePath
		filename = strings.TrimSuffix(filename, ext) + ".jpg"
		filePath = filepath.Join(uploadDir, filename)
		if err := os.Rename(oldPath, filePath); err != nil {
			return "", "", 0, 0, false, fmt.Errorf("Failed to rename file: %s", err.Error())
		}
	default:
		if err := jpeg.Encode(dst, img, &jpeg.Options{Quality: 85}); err != nil {
			return "", "", 0, 0, false, fmt.Errorf("Failed to encode image: %s", err.Error())
		}
		oldPath := filePath
		filename = strings.TrimSuffix(filename, ext) + ".jpg"
		filePath = filepath.Join(uploadDir, filename)
		if err := os.Rename(oldPath, filePath); err != nil {
			return "", "", 0, 0, false, fmt.Errorf("Failed to rename file: %s", err.Error())
		}
	}

	fileURL = fmt.Sprintf("/uploads/%s/%s/%s", tenantID.String(), time.Now().Format("2006-01-02"), filename)
	return fileURL, filePath, width, height, resized, nil
}

// UploadFile 處理文件上傳
func UploadFile(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	// 獲取上傳的文件
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "No file uploaded"})
	}

	fileURL, filePath, width, height, resized, err := saveUploadedImageForTenant(tenantID, file)
	if err != nil {
		// Keep the same public error shape as before
		msg := err.Error()
		status := 500
		if strings.HasPrefix(msg, "File size exceeds") ||
			strings.Contains(msg, "Only image files are allowed") ||
			strings.HasPrefix(msg, "Failed to decode") ||
			strings.HasPrefix(msg, "tenant_id is required") ||
			strings.HasPrefix(msg, "file is required") {
			status = 400
		}
		return c.Status(status).JSON(fiber.Map{"error": msg})
	}

	return c.JSON(fiber.Map{
		"url":     fileURL,
		"path":    filePath,
		"width":   width,
		"height":  height,
		"resized": resized,
	})
}
