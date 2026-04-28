package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// Server HTTP API 服務器
type Server struct {
	config     *Config
	httpServer *http.Server
	startTime  time.Time
}

// NewServer 創建新的服務器實例
func NewServer(cfg *Config) *Server {
	return &Server{
		config:    cfg,
		startTime: time.Now(),
	}
}

// Start 啟動 HTTP 服務器
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// 註冊路由
	mux.HandleFunc("/api/status", s.corsMiddleware(s.handleStatus))
	mux.HandleFunc("/api/printers", s.corsMiddleware(s.handleGetPrinters))
	mux.HandleFunc("/api/printers/test", s.corsMiddleware(s.handleTestPrinter))
	mux.HandleFunc("/api/printers/print", s.corsMiddleware(s.handlePrint))
	mux.HandleFunc("/api/thermal-printers", s.corsMiddleware(s.handleGetThermalPrinters))
	mux.HandleFunc("/api/thermal-printers/print", s.corsMiddleware(s.handleThermalPrint))
	mux.HandleFunc("/api/card-terminals", s.corsMiddleware(s.handleGetCardTerminals))
	mux.HandleFunc("/api/card-terminals/configure", s.corsMiddleware(s.handleConfigureCardTerminal))
	mux.HandleFunc("/api/card-terminals/test", s.corsMiddleware(s.handleTestCardTerminal))
	mux.HandleFunc("/api/card-terminals/payment", s.corsMiddleware(s.handlePayment))
	mux.HandleFunc("/api/card-terminals/cancel", s.corsMiddleware(s.handleCancelPayment))
	mux.HandleFunc("/api/card-terminals/query", s.corsMiddleware(s.handleQueryPayment))

	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf("127.0.0.1:%d", s.config.Port),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	log.Printf("vWork Connector starting on port %d", s.config.Port)
	return s.httpServer.ListenAndServe()
}

// Stop 停止服務器
func (s *Server) Stop() error {
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// CORS 中間件
func (s *Server) corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 只允許本地訪問
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}

// JSON 響應輔助函數
func (s *Server) jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (s *Server) jsonError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": false,
		"error":   message,
	})
}

// ===== API 處理器 =====

// handleStatus 狀態檢查
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	uptime := int64(time.Since(s.startTime).Seconds())
	s.jsonResponse(w, map[string]interface{}{
		"status":  "ok",
		"version": version,
		"uptime":  uptime,
	})
}

// handleGetPrinters 獲取打印機列表
func (s *Server) handleGetPrinters(w http.ResponseWriter, r *http.Request) {
	printers, err := GetSystemPrinters()
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.jsonResponse(w, map[string]interface{}{
		"printers": printers,
	})
}

// handleTestPrinter 測試打印機
func (s *Server) handleTestPrinter(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Printer string `json:"printer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	err := TestPrinter(req.Printer)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, map[string]interface{}{
		"success": true,
		"message": "測試頁已發送",
	})
}

// handlePrint 執行打印
func (s *Server) handlePrint(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req PrintRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	jobID, err := Print(req)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, map[string]interface{}{
		"success": true,
		"jobId":   jobID,
	})
}

// handleGetThermalPrinters 獲取熱敏打印機
func (s *Server) handleGetThermalPrinters(w http.ResponseWriter, r *http.Request) {
	printers, err := GetThermalPrinters()
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.jsonResponse(w, map[string]interface{}{
		"printers": printers,
	})
}

// handleThermalPrint 熱敏打印
func (s *Server) handleThermalPrint(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ThermalPrintRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	err := ThermalPrint(req)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, map[string]interface{}{
		"success": true,
		"message": "打印成功",
	})
}

// handleGetCardTerminals 獲取卡機列表
func (s *Server) handleGetCardTerminals(w http.ResponseWriter, r *http.Request) {
	s.jsonResponse(w, map[string]interface{}{
		"terminals": s.config.CardTerminals,
	})
}

// handleConfigureCardTerminal 配置卡機
func (s *Server) handleConfigureCardTerminal(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CardTerminalConfig
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// 生成 ID
	req.ID = fmt.Sprintf("terminal-%d", time.Now().UnixNano())

	// 保存到配置
	s.config.CardTerminals = append(s.config.CardTerminals, req)
	s.config.Save()

	s.jsonResponse(w, map[string]interface{}{
		"success":    true,
		"terminalId": req.ID,
	})
}

// handleTestCardTerminal 測試卡機連接
func (s *Server) handleTestCardTerminal(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		TerminalID string `json:"terminalId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// 查找卡機配置
	var terminal *CardTerminalConfig
	for i := range s.config.CardTerminals {
		if s.config.CardTerminals[i].ID == req.TerminalID {
			terminal = &s.config.CardTerminals[i]
			break
		}
	}

	if terminal == nil {
		s.jsonError(w, "Terminal not found", http.StatusNotFound)
		return
	}

	info, err := TestCardTerminal(*terminal)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, map[string]interface{}{
		"success":      true,
		"message":      "連接正常",
		"terminalInfo": info,
	})
}

// handlePayment 發起支付
func (s *Server) handlePayment(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req PaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// 查找卡機
	var terminal *CardTerminalConfig
	for i := range s.config.CardTerminals {
		if s.config.CardTerminals[i].ID == req.TerminalID {
			terminal = &s.config.CardTerminals[i]
			break
		}
	}

	if terminal == nil {
		s.jsonError(w, "Terminal not found", http.StatusNotFound)
		return
	}

	result, err := ProcessPayment(*terminal, req)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, result)
}

// handleCancelPayment 取消支付
func (s *Server) handleCancelPayment(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		TerminalID    string `json:"terminalId"`
		TransactionID string `json:"transactionId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// 查找卡機
	var terminal *CardTerminalConfig
	for i := range s.config.CardTerminals {
		if s.config.CardTerminals[i].ID == req.TerminalID {
			terminal = &s.config.CardTerminals[i]
			break
		}
	}

	if terminal == nil {
		s.jsonError(w, "Terminal not found", http.StatusNotFound)
		return
	}

	err := CancelPayment(*terminal, req.TransactionID)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, map[string]interface{}{
		"success": true,
		"message": "交易已取消",
	})
}

// handleQueryPayment 查詢支付狀態
func (s *Server) handleQueryPayment(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		TerminalID    string `json:"terminalId"`
		TransactionID string `json:"transactionId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// 查找卡機
	var terminal *CardTerminalConfig
	for i := range s.config.CardTerminals {
		if s.config.CardTerminals[i].ID == req.TerminalID {
			terminal = &s.config.CardTerminals[i]
			break
		}
	}

	if terminal == nil {
		s.jsonError(w, "Terminal not found", http.StatusNotFound)
		return
	}

	status, err := QueryPayment(*terminal, req.TransactionID)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, status)
}
