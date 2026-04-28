package handlers

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/gofiber/fiber/v2"
)

// DownloadConnectorZip 下載 vWork Connector ZIP 包
func DownloadConnectorZip(c *fiber.Ctx) error {
	// Connector exe 路徑
	connectorExePath := filepath.Join("dist", "vwork-connector", "vwork-connector.exe")
	
	if _, err := os.Stat(connectorExePath); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Connector not available"})
	}
	
	// 創建臨時 ZIP
	zipPath := filepath.Join(os.TempDir(), fmt.Sprintf("vwork-connector_%d.zip", time.Now().Unix()))
	
	if err := createConnectorZip(connectorExePath, zipPath); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to create ZIP: %v", err)})
	}
	defer os.Remove(zipPath)
	
	// 設置下載頭
	c.Set("Content-Type", "application/zip")
	c.Set("Content-Disposition", `attachment; filename="vwork-connector.zip"`)
	
	return c.SendFile(zipPath)
}

// createConnectorZip 創建 Connector ZIP 包
func createConnectorZip(exePath, zipPath string) error {
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer zipFile.Close()
	
	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()
	
	// 添加 exe 到 ZIP
	exeFile, err := os.Open(exePath)
	if err != nil {
		return err
	}
	defer exeFile.Close()
	
	exeInfo, err := exeFile.Stat()
	if err != nil {
		return err
	}
	
	header, err := zip.FileInfoHeader(exeInfo)
	if err != nil {
		return err
	}
	header.Name = "vwork-connector.exe"
	header.Method = zip.Deflate
	
	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		return err
	}
	
	_, err = io.Copy(writer, exeFile)
	return err
}
