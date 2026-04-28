package utils

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/phpdave11/gofpdf"
)

// findFontInDir 遞歸搜索目錄中的中文字體文件
func findFontInDir(dir string, extensions []string) string {
	if _, err := os.Stat(dir); err != nil {
		return ""
	}

	var found string
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		for _, wantedExt := range extensions {
			if ext == wantedExt {
				// 檢查文件名是否包含中文字體相關關鍵字
				baseName := strings.ToLower(filepath.Base(path))
				if strings.Contains(baseName, "noto") ||
					strings.Contains(baseName, "cjk") ||
					strings.Contains(baseName, "sans") ||
					strings.Contains(baseName, "serif") ||
					strings.Contains(baseName, "simsun") ||
					strings.Contains(baseName, "ming") ||
					strings.Contains(baseName, "hei") ||
					strings.Contains(baseName, "kai") {
					found = path
					return filepath.SkipAll // 找到一個就返回
				}
			}
		}
		return nil
	})
	return found
}

// FindCJKFontPath 嘗試在常見系統字體路徑找可用的中文字體（避免 repo 內放字體檔）
func FindCJKFontPath() string {
	// Windows：優先 Noto（可覆蓋繁中/簡中），其次 SimSun
	candidates := []string{}
	if runtime.GOOS == "windows" {
		winFonts := filepath.Join(os.Getenv("WINDIR"), "Fonts")
		candidates = append(candidates,
			filepath.Join(winFonts, "NotoSansTC-VF.ttf"),
			filepath.Join(winFonts, "NotoSansHK-VF.ttf"),
			filepath.Join(winFonts, "NotoSerifTC-VF.ttf"),
			filepath.Join(winFonts, "NotoSerifHK-VF.ttf"),
			filepath.Join(winFonts, "simsunb.ttf"),
			filepath.Join(winFonts, "SimsunExtG.ttf"),
		)
	} else {
		// Linux 常見路徑（擴展更多位置）
		linuxPaths := []string{
			"/usr/share/fonts/truetype/noto",
			"/usr/share/fonts/opentype/noto",
			"/usr/share/fonts/truetype",
			"/usr/share/fonts/opentype",
			"/usr/share/fonts",
			"/usr/local/share/fonts",
			filepath.Join(os.Getenv("HOME"), ".fonts"),
			filepath.Join(os.Getenv("HOME"), ".local/share/fonts"),
		}

		// 先檢查常見的具體路徑
		candidates = append(candidates,
			"/usr/share/fonts/truetype/noto/NotoSansTC-VF.ttf",
			"/usr/share/fonts/truetype/noto/NotoSansSC-VF.ttf",
			"/usr/share/fonts/truetype/noto/NotoSansHK-VF.ttf",
			"/usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttc",
			"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
			"/usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttf",
			"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttf",
			"/usr/share/fonts/truetype/noto/NotoSansTC-Regular.otf",
			"/usr/share/fonts/opentype/noto/NotoSansTC-Regular.otf",
			"/usr/share/fonts/truetype/noto/NotoSansSC-Regular.otf",
			"/usr/share/fonts/opentype/noto/NotoSansSC-Regular.otf",
			"/usr/share/fonts/truetype/noto/NotoSansHK-Regular.otf",
			"/usr/share/fonts/opentype/noto/NotoSansHK-Regular.otf",
			"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		)

		// 如果具體路徑找不到，遞歸搜索
		extensions := []string{".ttf", ".ttc", ".otf"}
		for _, dir := range linuxPaths {
			if found := findFontInDir(dir, extensions); found != "" {
				candidates = append(candidates, found)
			}
		}
	}

	// 檢查候選字體
	for _, p := range candidates {
		if p == "" {
			continue
		}

		// 清理路徑
		p = strings.TrimSpace(p)
		// 修正 Linux 上缺少前導斜線的路徑
		if runtime.GOOS != "windows" && strings.HasPrefix(p, "usr/") {
			p = "/" + p
		}

		// 確保路徑是絕對路徑
		var absPath string
		var err error
		if filepath.IsAbs(p) {
			// 已經是絕對路徑，清理後使用
			absPath = filepath.Clean(p)
		} else {
			// 轉換為絕對路徑
			absPath, err = filepath.Abs(p)
			if err != nil {
				continue
			}
			absPath = filepath.Clean(absPath)
		}

		// 對於 Linux，確保路徑以 / 開頭
		if runtime.GOOS != "windows" {
			if !strings.HasPrefix(absPath, "/") {
				// 如果是相對路徑轉換失敗，跳過
				continue
			}
		}

		// 再次驗證文件是否存在且可讀
		info, err := os.Stat(absPath)
		if err != nil {
			continue
		}
		if info.IsDir() {
			continue
		}

		// 確保返回的是正確格式的絕對路徑
		// 對於 Linux，使用正斜杠確保兼容性
		if runtime.GOOS != "windows" {
			absPath = filepath.ToSlash(absPath)
			// 再次確認以 / 開頭
			if !strings.HasPrefix(absPath, "/") {
				continue
			}
		}

		return absPath
	}
	return ""
}

func newPDFWithCJKFont(orientation string) (*gofpdf.Fpdf, string, error) {
	if orientation == "" {
		orientation = "P"
	}
	pdf := gofpdf.New(orientation, "pt", "A4", "")
	pdf.SetMargins(36, 36, 36)
	pdf.SetAutoPageBreak(true, 36)

	fontPath := FindCJKFontPath()
	fontName := "NotoCJK"

	if fontPath == "" {
		// 沒找到任何 CJK 字體，直接使用 Helvetica
		fontName = "Helvetica"
		pdf.SetFont(fontName, "", 10)
		return pdf, fontName, fmt.Errorf("no usable CJK font found")
	}

	// 清理並正規化路徑
	fontPath = strings.TrimSpace(fontPath)

	// 修正 Linux 上缺少前導斜線的路徑
	if runtime.GOOS != "windows" && !strings.HasPrefix(fontPath, "/") {
		if strings.HasPrefix(fontPath, "usr/") {
			fontPath = "/" + fontPath
		} else {
			// 非標準路徑，使用 fallback
			fontName = "Helvetica"
			pdf.SetFont(fontName, "", 10)
			return pdf, fontName, fmt.Errorf("no usable CJK font found")
		}
	}

	// 確保路徑是絕對路徑
	var absPath string
	var err error
	if filepath.IsAbs(fontPath) {
		absPath = filepath.Clean(fontPath)
	} else {
		absPath, err = filepath.Abs(fontPath)
		if err != nil {
			fontName = "Helvetica"
			pdf.SetFont(fontName, "", 10)
			return pdf, fontName, fmt.Errorf("no usable CJK font found")
		}
		absPath = filepath.Clean(absPath)
	}

	// 對於 Linux，確保路徑以 / 開頭
	if runtime.GOOS != "windows" {
		absPath = filepath.ToSlash(absPath)
		if !strings.HasPrefix(absPath, "/") {
			absPath = "/" + absPath
		}
	}

	// 驗證文件是否存在
	info, err := os.Stat(absPath)
	if err != nil || info.IsDir() {
		fontName = "Helvetica"
		pdf.SetFont(fontName, "", 10)
		return pdf, fontName, fmt.Errorf("no usable CJK font found")
	}

	ext := strings.ToLower(filepath.Ext(absPath))
	if ext != ".ttf" && ext != ".ttc" && ext != ".otf" {
		fontName = "Helvetica"
		pdf.SetFont(fontName, "", 10)
		return pdf, fontName, fmt.Errorf("no usable CJK font found")
	}

	// 最終防線：確保 Linux 絕對路徑以 / 開頭
	if runtime.GOOS != "windows" && !strings.HasPrefix(absPath, "/") {
		absPath = "/" + absPath
	}
	log.Printf("[PDF] newPDFWithCJKFont: using font path: %s", absPath)

	// 直接讀取字體檔案內容，避免 gofpdf 內部路徑處理問題
	fontBytes, err := os.ReadFile(absPath)
	if err != nil {
		log.Printf("[PDF] Failed to read font file: %v", err)
		fontName = "Helvetica"
		pdf.SetFont(fontName, "", 10)
		return pdf, fontName, fmt.Errorf("failed to read font file: %v", err)
	}
	pdf.AddUTF8FontFromBytes(fontName, "", fontBytes)
	pdf.SetFont(fontName, "", 10)
	return pdf, fontName, nil
}

// BuildTablePDFBytes 產生簡單表格 PDF（通用匯出用）
func BuildTablePDFBytes(title string, headers []string, rows [][]string) ([]byte, error) {
	// 欄位多就橫向
	orientation := "P"
	if len(headers) > 6 {
		orientation = "L"
	}
	pdf, fontName, fontErr := newPDFWithCJKFont(orientation)

	pdf.AddPage()
	pdf.SetFont(fontName, "", 14)
	pdf.CellFormat(0, 18, title, "", 1, "L", false, 0, "")
	pdf.Ln(6)
	pdf.SetFont(fontName, "", 10)

	pageW, _ := pdf.GetPageSize()
	left, _, right, _ := pdf.GetMargins()
	usableW := pageW - left - right

	// 每欄等寬（簡化）
	colW := usableW / float64(max(1, len(headers)))
	lineH := 14.0

	// header
	pdf.SetFillColor(240, 240, 240)
	pdf.SetDrawColor(200, 200, 200)
	for _, h := range headers {
		pdf.CellFormat(colW, lineH, h, "1", 0, "L", true, 0, "")
	}
	pdf.Ln(-1)
	pdf.SetFillColor(255, 255, 255)

	// rows
	for _, r := range rows {
		for i := 0; i < len(headers); i++ {
			val := ""
			if i < len(r) {
				val = r[i]
			}
			pdf.CellFormat(colW, lineH, val, "1", 0, "L", false, 0, "")
		}
		pdf.Ln(-1)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	if fontErr != nil {
		// 字體找不到時仍輸出 PDF（但提示由呼叫端決定要不要處理）
	}
	return buf.Bytes(), nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
