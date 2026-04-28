package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// hideWindow 設置命令為隱藏窗口模式
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}

// PrinterInfo 打印機信息
type PrinterInfo struct {
	Name    string `json:"name"`
	Driver  string `json:"driver"`
	Port    string `json:"port"`
	Default bool   `json:"default"`
	Status  string `json:"status"`
}

// PowerShellPrinter PowerShell 返回的打印機結構
type PowerShellPrinter struct {
	Name       string `json:"Name"`
	DriverName string `json:"DriverName"`
	PortName   string `json:"PortName"`
	Default    bool   `json:"Default"`
}

// ThermalPrinterInfo 熱敏打印機信息
type ThermalPrinterInfo struct {
	Name       string `json:"name"`
	Port       string `json:"port"`
	Type       string `json:"type"` // serial, network, usb
	PaperWidth int    `json:"paperWidth"`
}

// PrintRequest 打印請求
type PrintRequest struct {
	Printer string       `json:"printer"`
	Content string       `json:"content"`
	Options PrintOptions `json:"options"`
}

// PrintOptions 打印選項
type PrintOptions struct {
	Copies      int    `json:"copies"`
	Orientation string `json:"orientation"`
	PaperSize   string `json:"paperSize"`
}

// ThermalPrintRequest 熱敏打印請求
type ThermalPrintRequest struct {
	Printer string     `json:"printer"`
	Ticket  TicketData `json:"ticket"`
}

// TicketData 票據數據
type TicketData struct {
	TicketNumber  string `json:"ticketNumber"`
	PartySize     int    `json:"partySize"`
	AreaName      string `json:"areaName"`
	StoreName     string `json:"storeName"`
	Time          string `json:"time"`
	FooterMessage string `json:"footerMessage"`
	ShowQRCode    bool   `json:"showQRCode"`
	QRCodeURL     string `json:"qrCodeUrl"`
}

// GetSystemPrinters 獲取系統打印機列表
func GetSystemPrinters() ([]PrinterInfo, error) {
	// 使用 PowerShell 獲取打印機列表，輸出為 JSON
	psScript := `
$defaultPrinter = (Get-WmiObject -Query "SELECT * FROM Win32_Printer WHERE Default=$true" -ErrorAction SilentlyContinue).Name
Get-Printer | ForEach-Object {
    [PSCustomObject]@{
        Name = $_.Name
        DriverName = $_.DriverName
        PortName = $_.PortName
        Default = ($_.Name -eq $defaultPrinter)
    }
} | ConvertTo-Json -Compress
`
	cmd := exec.Command("powershell", "-NoProfile", "-Command", psScript)
	hideWindow(cmd)
	output, err := cmd.Output()
	if err != nil {
		// 如果 Get-Printer 失敗，嘗試 WMI 方式
		return getSystemPrintersWMI()
	}

	outputStr := strings.TrimSpace(string(output))
	if outputStr == "" || outputStr == "null" {
		return []PrinterInfo{}, nil
	}

	// PowerShell 單個結果會返回 object，多個返回 array
	var printers []PrinterInfo

	// 嘗試解析為數組
	var psPrinters []PowerShellPrinter
	if err := json.Unmarshal([]byte(outputStr), &psPrinters); err != nil {
		// 嘗試解析為單個對象
		var singlePrinter PowerShellPrinter
		if err := json.Unmarshal([]byte(outputStr), &singlePrinter); err != nil {
			// JSON 解析失敗，使用備用方式
			return getSystemPrintersSimple()
		}
		psPrinters = []PowerShellPrinter{singlePrinter}
	}

	for _, p := range psPrinters {
		printers = append(printers, PrinterInfo{
			Name:    p.Name,
			Driver:  p.DriverName,
			Port:    p.PortName,
			Default: p.Default,
			Status:  "ready",
		})
	}

	return printers, nil
}

// getSystemPrintersWMI 使用 WMI 獲取打印機
func getSystemPrintersWMI() ([]PrinterInfo, error) {
	psScript := `
Get-WmiObject -Class Win32_Printer | ForEach-Object {
    [PSCustomObject]@{
        Name = $_.Name
        DriverName = $_.DriverName
        PortName = $_.PortName
        Default = $_.Default
    }
} | ConvertTo-Json -Compress
`
	cmd := exec.Command("powershell", "-NoProfile", "-Command", psScript)
	hideWindow(cmd)
	output, err := cmd.Output()
	if err != nil {
		return getSystemPrintersSimple()
	}

	outputStr := strings.TrimSpace(string(output))
	if outputStr == "" || outputStr == "null" {
		return []PrinterInfo{}, nil
	}

	var printers []PrinterInfo
	var psPrinters []PowerShellPrinter
	if err := json.Unmarshal([]byte(outputStr), &psPrinters); err != nil {
		var singlePrinter PowerShellPrinter
		if err := json.Unmarshal([]byte(outputStr), &singlePrinter); err != nil {
			return getSystemPrintersSimple()
		}
		psPrinters = []PowerShellPrinter{singlePrinter}
	}

	for _, p := range psPrinters {
		printers = append(printers, PrinterInfo{
			Name:    p.Name,
			Driver:  p.DriverName,
			Port:    p.PortName,
			Default: p.Default,
			Status:  "ready",
		})
	}

	return printers, nil
}

// getSystemPrintersSimple 簡單方式獲取打印機名稱
func getSystemPrintersSimple() ([]PrinterInfo, error) {
	cmd := exec.Command("powershell", "-NoProfile", "-Command", `(Get-Printer).Name`)
	hideWindow(cmd)
	output, err := cmd.Output()
	if err != nil {
		// 最後嘗試 wmic
		return getSystemPrintersWMIC()
	}

	var printers []PrinterInfo
	names := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name != "" {
			printers = append(printers, PrinterInfo{
				Name:   name,
				Status: "ready",
			})
		}
	}

	return printers, nil
}

// getSystemPrintersWMIC 使用 wmic 命令獲取打印機
func getSystemPrintersWMIC() ([]PrinterInfo, error) {
	cmd := exec.Command("wmic", "printer", "get", "name", "/format:list")
	hideWindow(cmd)
	output, err := cmd.Output()
	if err != nil {
		return []PrinterInfo{}, fmt.Errorf("failed to get printers: %v", err)
	}

	var printers []PrinterInfo
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Name=") {
			name := strings.TrimPrefix(line, "Name=")
			name = strings.TrimSpace(name)
			if name != "" {
				printers = append(printers, PrinterInfo{
					Name:   name,
					Status: "ready",
				})
			}
		}
	}

	return printers, nil
}

// TestPrinter 測試打印機
func TestPrinter(printerName string) error {
	// 檢查打印機是否存在
	psScript := fmt.Sprintf(`
$printer = Get-Printer -Name "%s" -ErrorAction SilentlyContinue
if ($printer) {
    Write-Output "OK"
} else {
    Write-Error "Printer not found"
    exit 1
}
`, printerName)

	cmd := exec.Command("powershell", "-NoProfile", "-Command", psScript)
	hideWindow(cmd)
	_, err := cmd.Output()
	return err
}

// Print 執行打印
func Print(req PrintRequest) (string, error) {
	jobID := fmt.Sprintf("job-%d", time.Now().UnixMilli())

	// 創建臨時文件並打印
	// 使用 Out-Printer 直接發送到打印機（適用於熱敏打印機）
	psScript := fmt.Sprintf(`
$content = @'
%s
'@
$printerName = '%s'

# 嘗試使用 Out-Printer 直接打印純文本
try {
    # 先移除 HTML 標籤，轉換為純文本
    $plainText = $content -replace '<[^>]+>', '' -replace '&nbsp;', ' ' -replace '&lt;', '<' -replace '&gt;', '>' -replace '&amp;', '&'
    $plainText = $plainText.Trim()
    
    if ($printerName -and $printerName -ne '') {
        $plainText | Out-Printer -Name $printerName
    } else {
        $plainText | Out-Printer
    }
    exit 0
} catch {
    # 如果 Out-Printer 失敗，嘗試使用默認關聯程序打印
    $tempFile = [System.IO.Path]::GetTempFileName() -replace '\.tmp$', '.txt'
    $content -replace '<[^>]+>', '' | Out-File -FilePath $tempFile -Encoding UTF8
    
    if ($printerName -and $printerName -ne '') {
        Start-Process $tempFile -Verb PrintTo -ArgumentList ('"{0}"' -f $printerName) -Wait -WindowStyle Hidden
    } else {
        Start-Process $tempFile -Verb Print -Wait -WindowStyle Hidden
    }
    
    Start-Sleep -Seconds 2
    Remove-Item $tempFile -ErrorAction SilentlyContinue
}
`, req.Content, req.Printer)

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", psScript)
	hideWindow(cmd)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("print failed: %v", err)
	}

	return jobID, nil
}

// GetThermalPrinters 獲取熱敏打印機列表
func GetThermalPrinters() ([]ThermalPrinterInfo, error) {
	// 獲取系統打印機中可能是熱敏打印機的設備
	// 通常熱敏打印機名稱包含 EPSON, Star, Citizen, Bixolon 等

	allPrinters, err := GetSystemPrinters()
	if err != nil {
		return nil, err
	}

	thermalKeywords := []string{"EPSON", "TM-", "Star", "TSP", "Citizen", "Bixolon", "POS", "Receipt", "Thermal"}
	thermalPrinters := []ThermalPrinterInfo{}

	for _, p := range allPrinters {
		isThermal := false
		for _, keyword := range thermalKeywords {
			if strings.Contains(strings.ToUpper(p.Name), strings.ToUpper(keyword)) {
				isThermal = true
				break
			}
		}

		if isThermal {
			thermalPrinters = append(thermalPrinters, ThermalPrinterInfo{
				Name:       p.Name,
				Port:       p.Port,
				Type:       "system",
				PaperWidth: 80, // 默認 80mm
			})
		}
	}

	return thermalPrinters, nil
}

// ThermalPrint 熱敏打印
func ThermalPrint(req ThermalPrintRequest) error {
	// 生成票據內容
	content := generateTicketContent(req.Ticket)

	// 使用系統打印
	printReq := PrintRequest{
		Printer: req.Printer,
		Content: content,
		Options: PrintOptions{
			Copies: 1,
		},
	}

	_, err := Print(printReq)
	return err
}

// generateTicketContent 生成票據純文本（適合熱敏打印機）
func generateTicketContent(ticket TicketData) string {
	// 使用純文本格式，適合熱敏打印機
	lines := []string{
		"================================",
		"",
		centerText(ticket.StoreName, 32),
		"",
		"--------------------------------",
		"",
		centerText(ticket.TicketNumber, 32),
		"",
		fmt.Sprintf("    人數: %d 位", ticket.PartySize),
		"",
		fmt.Sprintf("    區域: %s", ticket.AreaName),
		"",
		fmt.Sprintf("    時間: %s", ticket.Time),
		"",
		"--------------------------------",
	}

	if ticket.FooterMessage != "" {
		lines = append(lines, "", centerText(ticket.FooterMessage, 32))
	}

	lines = append(lines, "", "================================", "", "", "")

	return strings.Join(lines, "\n")
}

// centerText 居中文字
func centerText(text string, width int) string {
	textLen := len([]rune(text))
	if textLen >= width {
		return text
	}
	padding := (width - textLen) / 2
	return strings.Repeat(" ", padding) + text
}
