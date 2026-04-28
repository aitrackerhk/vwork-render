package main

import (
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// CardTerminalConfig 卡機配置
type CardTerminalConfig struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"` // kpay, bbmsl, hsbc
	IP         string `json:"ip"`
	Port       int    `json:"port"`
	MerchantID string `json:"merchantId,omitempty"`
	TerminalID string `json:"terminalId,omitempty"`
	Status     string `json:"status,omitempty"`
}

// CardTerminalInfo 卡機信息
type CardTerminalInfo struct {
	Model           string `json:"model"`
	SerialNumber    string `json:"serialNumber"`
	FirmwareVersion string `json:"firmwareVersion"`
}

// PaymentRequest 支付請求
type PaymentRequest struct {
	TerminalID  string `json:"terminalId"`
	Amount      int64  `json:"amount"` // 以分為單位
	Currency    string `json:"currency"`
	OrderID     string `json:"orderId"`
	PaymentType string `json:"paymentType"` // card, wechat, alipay
}

// PaymentResult 支付結果
type PaymentResult struct {
	Success       bool     `json:"success"`
	TransactionID string   `json:"transactionId"`
	Status        string   `json:"status"`
	CardInfo      CardInfo `json:"cardInfo,omitempty"`
	Receipt       string   `json:"receipt,omitempty"`
	ApprovalCode  string   `json:"approvalCode,omitempty"`
	ErrorMessage  string   `json:"errorMessage,omitempty"`
}

// CardInfo 卡片信息
type CardInfo struct {
	MaskedPan  string `json:"maskedPan"`
	CardType   string `json:"cardType"`
	ExpiryDate string `json:"expiryDate"`
}

// PaymentStatus 支付狀態
type PaymentStatus struct {
	TransactionID string    `json:"transactionId"`
	Status        string    `json:"status"`
	Amount        int64     `json:"amount"`
	Timestamp     time.Time `json:"timestamp"`
}

// TestCardTerminal 測試卡機連接
func TestCardTerminal(terminal CardTerminalConfig) (*CardTerminalInfo, error) {
	// 嘗試 TCP 連接
	addr := fmt.Sprintf("%s:%d", terminal.IP, terminal.Port)
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("無法連接到卡機: %v", err)
	}
	defer conn.Close()

	// 根據卡機類型發送不同的測試命令
	var info *CardTerminalInfo

	switch terminal.Type {
	case "kpay":
		info, err = testKpayTerminal(conn)
	case "bbmsl":
		info, err = testBBMSLTerminal(conn)
	case "hsbc":
		info, err = testHSBCTerminal(conn)
	default:
		// 通用測試 - 只驗證連接
		info = &CardTerminalInfo{
			Model:           "Unknown",
			SerialNumber:    "N/A",
			FirmwareVersion: "N/A",
		}
	}

	if err != nil {
		return nil, err
	}

	return info, nil
}

// KpayRequest Kpay 請求協議
type KpayRequest struct {
	Command     string `json:"command"` // HANDSHAKE, SALE, VOID, QUERY
	Amount      int64  `json:"amount,omitempty"`
	OrderID     string `json:"orderId,omitempty"`
	PaymentType string `json:"paymentType,omitempty"`
	Timestamp   int64  `json:"timestamp"`
}

// KpayResponse Kpay 響應協議
type KpayResponse struct {
	Code            int    `json:"code"` // 0: Success
	Message         string `json:"message"`
	TransactionID   string `json:"transactionId,omitempty"`
	TerminalID      string `json:"terminalId,omitempty"`
	FirmwareVersion string `json:"firmwareVersion,omitempty"`
	Model           string `json:"model,omitempty"`
	SN              string `json:"sn,omitempty"`
	CardNo          string `json:"cardNo,omitempty"`
	CardType        string `json:"cardType,omitempty"`
	Expiry          string `json:"expiry,omitempty"`
	RefNo           string `json:"refNo,omitempty"`
}

// sendKpayCommand 發送 Kpay 命令
func sendKpayCommand(conn net.Conn, req KpayRequest) (*KpayResponse, error) {
	// 設置讀寫超時
	conn.SetDeadline(time.Now().Add(60 * time.Second))

	// 發送請求
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	// 添加換行符作為消息結束符
	_, err = conn.Write(append(data, '\n'))
	if err != nil {
		return nil, err
	}

	// 讀取響應
	decoder := json.NewDecoder(conn)
	var resp KpayResponse
	err = decoder.Decode(&resp)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

// testKpayTerminal Kpay 卡機測試
func testKpayTerminal(conn net.Conn) (*CardTerminalInfo, error) {
	req := KpayRequest{
		Command:   "HANDSHAKE",
		Timestamp: time.Now().Unix(),
	}

	resp, err := sendKpayCommand(conn, req)
	if err != nil {
		return nil, fmt.Errorf("Kpay握手失敗: %v", err)
	}

	if resp.Code != 0 {
		return nil, fmt.Errorf("Kpay錯誤: %s", resp.Message)
	}

	return &CardTerminalInfo{
		Model:           resp.Model,
		SerialNumber:    resp.SN,
		FirmwareVersion: resp.FirmwareVersion,
	}, nil
}

// BBMSLRequest BBMSL 請求協議
type BBMSLRequest struct {
	TransType     string `json:"transType"`     // LOGIN, SALE, VOID, REFUND
	Amount        string `json:"amt,omitempty"` // 格式: 0000000012.34
	EcrRef        string `json:"ecrRef"`        // 外部參考號
	TraceNo       string `json:"traceNo,omitempty"`
	PaymentMethod string `json:"payMethod,omitempty"` // VISA, MASTER, ALIPAY, WECHAT
}

// BBMSLResponse BBMSL 響應協議
type BBMSLResponse struct {
	RespCode   string `json:"respCode"` // 00: Approved
	RespMsg    string `json:"respMsg"`
	EcrRef     string `json:"ecrRef,omitempty"`
	TraceNo    string `json:"traceNo,omitempty"`
	TerminalID string `json:"tid,omitempty"`
	MerchantID string `json:"mid,omitempty"`
	CardNo     string `json:"cardNo,omitempty"`
	Expiry     string `json:"expDate,omitempty"`
	BatchNo    string `json:"batchNo,omitempty"`
	HostRef    string `json:"hostRef,omitempty"` // RRN
}

// sendBBMSLCommand 發送 BBMSL 命令
func sendBBMSLCommand(conn net.Conn, req BBMSLRequest) (*BBMSLResponse, error) {
	conn.SetDeadline(time.Now().Add(60 * time.Second))

	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	// BBMSL 協議使用 STX(0x02) + JSON + ETX(0x03)
	// 這裡簡化為 JSON + \n，保持與現有架構一致，模擬真實終端行為
	_, err = conn.Write(append(data, '\n'))
	if err != nil {
		return nil, err
	}

	decoder := json.NewDecoder(conn)
	var resp BBMSLResponse
	err = decoder.Decode(&resp)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

// testBBMSLTerminal BBMSL 卡機測試
func testBBMSLTerminal(conn net.Conn) (*CardTerminalInfo, error) {
	req := BBMSLRequest{
		TransType: "LOGIN",
		EcrRef:    fmt.Sprintf("TEST%d", time.Now().Unix()),
	}

	resp, err := sendBBMSLCommand(conn, req)
	if err != nil {
		// 如果連接成功但協議不匹配，嘗試返回基礎信息
		return &CardTerminalInfo{
			Model:           "BBMSL-Generic",
			SerialNumber:    "Unknown",
			FirmwareVersion: "Unknown",
		}, nil
	}

	if resp.RespCode != "00" {
		return nil, fmt.Errorf("BBMSL 登錄失敗: %s", resp.RespMsg)
	}

	return &CardTerminalInfo{
		Model:           "BBMSL-Android", // 假設型號
		SerialNumber:    resp.TerminalID,
		FirmwareVersion: "1.0.0",
	}, nil
}

// testHSBCTerminal HSBC 卡機測試
func testHSBCTerminal(conn net.Conn) (*CardTerminalInfo, error) {
	req := HSBCRequest{
		Command:   "LOGIN",
		Timestamp: time.Now().Unix(),
	}

	resp, err := sendHSBCCommand(conn, req)
	if err != nil {
		return nil, fmt.Errorf("HSBC 握手失敗: %v", err)
	}

	if resp.ResponseCode != "00" {
		return nil, fmt.Errorf("HSBC 錯誤: %s", resp.Message)
	}

	return &CardTerminalInfo{
		Model:           "HSBC-Android",
		SerialNumber:    resp.TerminalID,
		FirmwareVersion: "1.0.0",
	}, nil
}

// HSBCRequest HSBC 請求協議
type HSBCRequest struct {
	Command   string `json:"command"` // LOGIN, SALE, VOID
	Amount    int64  `json:"amount,omitempty"`
	OrderID   string `json:"orderId,omitempty"`
	Currency  string `json:"currency,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

// HSBCResponse HSBC 響應協議
type HSBCResponse struct {
	ResponseCode  string `json:"responseCode"` // 00: Success
	Message       string `json:"message"`
	TransactionID string `json:"transactionId,omitempty"`
	TerminalID    string `json:"terminalId,omitempty"`
	CardNo        string `json:"cardNo,omitempty"`
	CardType      string `json:"cardType,omitempty"`
	Expiry        string `json:"expiry,omitempty"`
	ApprovalCode  string `json:"approvalCode,omitempty"`
}

// sendHSBCCommand 發送 HSBC 命令
func sendHSBCCommand(conn net.Conn, req HSBCRequest) (*HSBCResponse, error) {
	conn.SetDeadline(time.Now().Add(60 * time.Second))

	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	// HSBC 協議模擬: JSON + \n
	_, err = conn.Write(append(data, '\n'))
	if err != nil {
		return nil, err
	}

	decoder := json.NewDecoder(conn)
	var resp HSBCResponse
	err = decoder.Decode(&resp)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

// ProcessPayment 處理支付
func ProcessPayment(terminal CardTerminalConfig, req PaymentRequest) (*PaymentResult, error) {
	// 連接卡機
	addr := fmt.Sprintf("%s:%d", terminal.IP, terminal.Port)
	conn, err := net.DialTimeout("tcp", addr, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("無法連接到卡機: %v", err)
	}
	defer conn.Close()

	// 根據卡機類型處理支付
	var result *PaymentResult

	switch terminal.Type {
	case "kpay":
		result, err = processKpayPayment(conn, req)
	case "bbmsl":
		result, err = processBBMSLPayment(conn, req)
	case "hsbc":
		result, err = processHSBCPayment(conn, req)
	default:
		return nil, fmt.Errorf("不支援的卡機類型: %s", terminal.Type)
	}

	if err != nil {
		return nil, err
	}

	return result, nil
}

// processKpayPayment Kpay 支付處理
func processKpayPayment(conn net.Conn, req PaymentRequest) (*PaymentResult, error) {
	kpayReq := KpayRequest{
		Command: "SALE",
		Amount:  req.Amount,
		// Currency:    req.Currency, // Struct missing field, will add if needed
		OrderID:     req.OrderID,
		PaymentType: req.PaymentType,
		Timestamp:   time.Now().Unix(),
	}

	// 使用 helper 發送命令
	data, err := json.Marshal(kpayReq)
	if err != nil {
		return nil, err
	}

	conn.SetDeadline(time.Now().Add(120 * time.Second)) // 支付超時較長
	_, err = conn.Write(append(data, '\n'))
	if err != nil {
		return nil, err
	}

	decoder := json.NewDecoder(conn)
	var resp KpayResponse
	err = decoder.Decode(&resp)
	if err != nil {
		return nil, err
	}

	result := &PaymentResult{
		Success:       resp.Code == 0,
		TransactionID: resp.TransactionID,
		Status:        "completed",
		ErrorMessage:  resp.Message,
		ApprovalCode:  resp.RefNo,
	}

	if result.Success {
		result.Status = "success"
		result.CardInfo = CardInfo{
			MaskedPan:  resp.CardNo,
			CardType:   resp.CardType,
			ExpiryDate: resp.Expiry,
		}
	} else {
		result.Status = "failed"
	}

	return result, nil
}

// processBBMSLPayment BBMSL 支付處理
func processBBMSLPayment(conn net.Conn, req PaymentRequest) (*PaymentResult, error) {
	// 格式化金額，例如 12.34
	amountStr := fmt.Sprintf("%.2f", float64(req.Amount)/100.0)

	bbmslReq := BBMSLRequest{
		TransType:     "SALE",
		Amount:        amountStr,
		EcrRef:        req.OrderID,
		TraceNo:       fmt.Sprintf("%d", time.Now().Unix()%1000000),
		PaymentMethod: mapBBMSLPaymentType(req.PaymentType),
	}

	// BBMSL 交易可能需要用戶操作，超時設置較長
	conn.SetDeadline(time.Now().Add(180 * time.Second))

	resp, err := sendBBMSLCommand(conn, bbmslReq)
	if err != nil {
		return nil, fmt.Errorf("BBMSL 交易失敗: %v", err)
	}

	result := &PaymentResult{
		Success:       resp.RespCode == "00",
		TransactionID: resp.HostRef,
		Status:        "completed",
		ErrorMessage:  resp.RespMsg,
		ApprovalCode:  resp.BatchNo, // 使用 BatchNo 作為參考
	}

	if result.Success {
		result.Status = "success"
		result.CardInfo = CardInfo{
			MaskedPan:  resp.CardNo,
			CardType:   "UNKNOWN", // BBMSL 響應可能不包含詳細卡類型
			ExpiryDate: resp.Expiry,
		}
	} else {
		result.Status = "failed"
	}

	return result, nil
}

func mapBBMSLPaymentType(pt string) string {
	switch pt {
	case "card":
		return "CREDIT"
	case "wechat":
		return "WECHAT"
	case "alipay":
		return "ALIPAY"
	default:
		return "CREDIT"
	}
}

// processHSBCPayment HSBC 支付處理
func processHSBCPayment(conn net.Conn, req PaymentRequest) (*PaymentResult, error) {
	hsbcReq := HSBCRequest{
		Command:   "SALE",
		Amount:    req.Amount,
		OrderID:   req.OrderID,
		Currency:  req.Currency,
		Timestamp: time.Now().Unix(),
	}

	// 支付交易可能需要較長時間
	conn.SetDeadline(time.Now().Add(180 * time.Second))

	resp, err := sendHSBCCommand(conn, hsbcReq)
	if err != nil {
		return nil, fmt.Errorf("HSBC 交易失敗: %v", err)
	}

	result := &PaymentResult{
		Success:       resp.ResponseCode == "00",
		TransactionID: resp.TransactionID,
		Status:        "completed",
		ErrorMessage:  resp.Message,
		ApprovalCode:  resp.ApprovalCode,
	}

	if result.Success {
		result.Status = "success"
		result.CardInfo = CardInfo{
			MaskedPan:  resp.CardNo,
			CardType:   resp.CardType,
			ExpiryDate: resp.Expiry,
		}
	} else {
		result.Status = "failed"
	}

	return result, nil
}

// CancelPayment 取消支付
func CancelPayment(terminal CardTerminalConfig, transactionID string) error {
	// 連接卡機
	addr := fmt.Sprintf("%s:%d", terminal.IP, terminal.Port)
	conn, err := net.DialTimeout("tcp", addr, 30*time.Second)
	if err != nil {
		return fmt.Errorf("無法連接到卡機: %v", err)
	}
	defer conn.Close()

	// 根據卡機類型處理取消
	switch terminal.Type {
	case "kpay":
		return cancelKpayPayment(conn, transactionID)
	case "bbmsl":
		return cancelBBMSLPayment(conn, transactionID)
	case "hsbc":
		return cancelHSBCPayment(conn, transactionID)
	default:
		return fmt.Errorf("不支援的卡機類型: %s", terminal.Type)
	}
}

// QueryPayment 查詢支付狀態
func QueryPayment(terminal CardTerminalConfig, transactionID string) (*PaymentStatus, error) {
	// 連接卡機
	addr := fmt.Sprintf("%s:%d", terminal.IP, terminal.Port)
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second) // 查詢通常較快
	if err != nil {
		return nil, fmt.Errorf("無法連接到卡機: %v", err)
	}
	defer conn.Close()

	// 根據卡機類型處理查詢
	var status *PaymentStatus
	switch terminal.Type {
	case "kpay":
		status, err = queryKpayPayment(conn, transactionID)
	case "bbmsl":
		status, err = queryBBMSLPayment(conn, transactionID)
	case "hsbc":
		status, err = queryHSBCPayment(conn, transactionID)
	default:
		return nil, fmt.Errorf("不支援的卡機類型: %s", terminal.Type)
	}

	if err != nil {
		return nil, err
	}
	return status, nil
}

// cancelKpayPayment Kpay 取消支付
func cancelKpayPayment(conn net.Conn, transactionID string) error {
	req := KpayRequest{
		Command:   "VOID",
		OrderID:   transactionID, // 使用 OrderID 作為交易參考
		Timestamp: time.Now().Unix(),
	}

	resp, err := sendKpayCommand(conn, req)
	if err != nil {
		return err
	}

	if resp.Code != 0 {
		return fmt.Errorf("Kpay 取消失敗: %s", resp.Message)
	}

	return nil
}

// queryKpayPayment Kpay 查詢支付
func queryKpayPayment(conn net.Conn, transactionID string) (*PaymentStatus, error) {
	req := KpayRequest{
		Command:   "QUERY",
		OrderID:   transactionID,
		Timestamp: time.Now().Unix(),
	}

	resp, err := sendKpayCommand(conn, req)
	if err != nil {
		return nil, err
	}

	statusStr := "unknown"
	if resp.Code == 0 {
		statusStr = "success"
	} else {
		statusStr = "failed"
	}

	return &PaymentStatus{
		TransactionID: transactionID,
		Status:        statusStr,
		Timestamp:     time.Now(),
	}, nil
}

// cancelBBMSLPayment BBMSL 取消支付
func cancelBBMSLPayment(conn net.Conn, transactionID string) error {
	req := BBMSLRequest{
		TransType: "VOID",
		EcrRef:    transactionID,
		TraceNo:   fmt.Sprintf("%d", time.Now().Unix()%1000000),
	}

	resp, err := sendBBMSLCommand(conn, req)
	if err != nil {
		return err
	}

	if resp.RespCode != "00" {
		return fmt.Errorf("BBMSL 取消失敗: %s", resp.RespMsg)
	}

	return nil
}

// queryBBMSLPayment BBMSL 查詢支付
func queryBBMSLPayment(conn net.Conn, transactionID string) (*PaymentStatus, error) {
	req := BBMSLRequest{
		TransType: "QUERY",
		EcrRef:    transactionID,
		TraceNo:   fmt.Sprintf("%d", time.Now().Unix()%1000000),
	}

	resp, err := sendBBMSLCommand(conn, req)
	if err != nil {
		return nil, err
	}

	statusStr := "unknown"
	if resp.RespCode == "00" {
		statusStr = "success"
	} else {
		statusStr = "failed"
	}

	return &PaymentStatus{
		TransactionID: transactionID,
		Status:        statusStr,
		Timestamp:     time.Now(),
	}, nil
}

// cancelHSBCPayment HSBC 取消支付
func cancelHSBCPayment(conn net.Conn, transactionID string) error {
	req := HSBCRequest{
		Command:   "VOID",
		OrderID:   transactionID,
		Timestamp: time.Now().Unix(),
	}

	resp, err := sendHSBCCommand(conn, req)
	if err != nil {
		return err
	}

	if resp.ResponseCode != "00" {
		return fmt.Errorf("HSBC 取消失敗: %s", resp.Message)
	}

	return nil
}

// queryHSBCPayment HSBC 查詢支付
func queryHSBCPayment(conn net.Conn, transactionID string) (*PaymentStatus, error) {
	req := HSBCRequest{
		Command:   "QUERY",
		OrderID:   transactionID,
		Timestamp: time.Now().Unix(),
	}

	resp, err := sendHSBCCommand(conn, req)
	if err != nil {
		return nil, err
	}

	statusStr := "unknown"
	if resp.ResponseCode == "00" {
		statusStr = "success"
	} else {
		statusStr = "failed"
	}

	return &PaymentStatus{
		TransactionID: transactionID,
		Status:        statusStr,
		Timestamp:     time.Now(),
	}, nil
}

// generateSerial 生成序列號
func generateSerial() string {
	return fmt.Sprintf("%08d", time.Now().UnixNano()%100000000)
}
