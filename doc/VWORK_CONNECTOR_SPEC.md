# vWork Connector 規格文檔

## 概述

vWork Connector 是一個本地 Windows 應用程式（exe），作為 vWork 網頁應用與本地硬件設備之間的橋樑。它提供 HTTP API 服務，讓網頁可以：

1. **獲取系統打印機列表**
2. **連接和控制卡機**（如 Kpay）
3. **執行本地打印任務**
4. **管理熱敏打印機**

## 技術規格

### 基本配置

- **默認端口**: `9527`
- **協議**: HTTP REST API
- **數據格式**: JSON
- **支援平台**: Windows 10/11

### API 端點

#### 1. 狀態檢查

```
GET /api/status
```

**響應**:
```json
{
  "status": "ok",
  "version": "1.0.0",
  "uptime": 12345
}
```

---

#### 2. 打印機相關 API

##### 2.1 獲取打印機列表

```
GET /api/printers
```

**響應**:
```json
{
  "printers": [
    {
      "name": "HP LaserJet Pro",
      "driver": "HP Universal Print Driver",
      "port": "USB001",
      "default": true,
      "status": "ready"
    },
    {
      "name": "EPSON TM-T88V",
      "driver": "EPSON TM-T88V Receipt",
      "port": "COM3",
      "default": false,
      "status": "ready"
    }
  ]
}
```

##### 2.2 測試打印機

```
POST /api/printers/test
```

**請求**:
```json
{
  "printer": "HP LaserJet Pro"
}
```

**響應**:
```json
{
  "success": true,
  "message": "測試頁已發送"
}
```

##### 2.3 執行打印

```
POST /api/printers/print
```

**請求**:
```json
{
  "printer": "HP LaserJet Pro",
  "content": "<html>...</html>",
  "options": {
    "copies": 1,
    "orientation": "portrait",
    "paperSize": "A4"
  }
}
```

**響應**:
```json
{
  "success": true,
  "jobId": "12345"
}
```

---

#### 3. 卡機相關 API

##### 3.1 獲取卡機列表

```
GET /api/card-terminals
```

**響應**:
```json
{
  "terminals": [
    {
      "id": "terminal-001",
      "name": "收銀台卡機",
      "type": "kpay",
      "ip": "192.168.1.100",
      "port": 8080,
      "status": "connected"
    }
  ]
}
```

##### 3.2 配置卡機

```
POST /api/card-terminals/configure
```

**請求**:
```json
{
  "type": "kpay",
  "ip": "192.168.1.100",
  "port": 8080,
  "merchantId": "MERCHANT001",
  "terminalId": "TERM001"
}
```

**響應**:
```json
{
  "success": true,
  "terminalId": "terminal-001"
}
```

##### 3.3 測試卡機連接

```
POST /api/card-terminals/test
```

**請求**:
```json
{
  "terminalId": "terminal-001"
}
```

**響應**:
```json
{
  "success": true,
  "message": "連接正常",
  "terminalInfo": {
    "model": "K1",
    "serialNumber": "KP12345678",
    "firmwareVersion": "2.1.0"
  }
}
```

##### 3.4 發起支付

```
POST /api/card-terminals/payment
```

**請求**:
```json
{
  "terminalId": "terminal-001",
  "amount": 10000,
  "currency": "HKD",
  "orderId": "ORDER-12345",
  "paymentType": "card"
}
```

**響應**:
```json
{
  "success": true,
  "transactionId": "TXN-20260131-001",
  "status": "approved",
  "cardInfo": {
    "maskedPan": "************1234",
    "cardType": "VISA",
    "expiryDate": "12/28"
  },
  "receipt": "...",
  "approvalCode": "123456"
}
```

##### 3.5 取消支付

```
POST /api/card-terminals/cancel
```

**請求**:
```json
{
  "terminalId": "terminal-001",
  "transactionId": "TXN-20260131-001"
}
```

**響應**:
```json
{
  "success": true,
  "message": "交易已取消"
}
```

##### 3.6 查詢支付狀態

```
POST /api/card-terminals/query
```

**請求**:
```json
{
  "terminalId": "terminal-001",
  "transactionId": "TXN-20260131-001"
}
```

**響應**:
```json
{
  "transactionId": "TXN-20260131-001",
  "status": "approved",
  "amount": 10000,
  "timestamp": "2026-01-31T14:30:00Z"
}
```

---

#### 4. 熱敏打印機 API

##### 4.1 獲取熱敏打印機列表

```
GET /api/thermal-printers
```

**響應**:
```json
{
  "printers": [
    {
      "name": "EPSON TM-T88V",
      "port": "COM3",
      "type": "serial",
      "paperWidth": 80
    },
    {
      "name": "Network Printer",
      "ip": "192.168.1.50",
      "port": 9100,
      "type": "network",
      "paperWidth": 58
    }
  ]
}
```

##### 4.2 熱敏打印

```
POST /api/thermal-printers/print
```

**請求**:
```json
{
  "printer": "EPSON TM-T88V",
  "ticket": {
    "ticketNumber": "A-001",
    "partySize": 4,
    "areaName": "大廳區",
    "storeName": "測試店家",
    "time": "2026-01-31 14:30",
    "footerMessage": "感謝您的耐心等候",
    "showQRCode": true,
    "qrCodeUrl": "https://example.com/queue/A-001"
  }
}
```

**響應**:
```json
{
  "success": true,
  "message": "打印成功"
}
```

---

## 配置文件

vWork Connector 使用 `config.json` 儲存配置：

```json
{
  "port": 9527,
  "autoStart": true,
  "minimizeToTray": true,
  "logLevel": "info",
  "cardTerminals": [
    {
      "id": "terminal-001",
      "name": "收銀台卡機",
      "type": "kpay",
      "ip": "192.168.1.100",
      "port": 8080
    }
  ]
}
```

## 系統托盤功能

- 顯示連接狀態圖標
- 右鍵菜單：
  - 開啟設定
  - 查看日誌
  - 檢查更新
  - 退出

## 開發技術建議

### 建議使用的技術棧

1. **Rust + Tauri**: 
   - 輕量級、高性能
   - 與 vCapture 技術統一
   - 良好的 Windows API 支援

2. **Go + Wails**:
   - 與 vWork 後端技術統一
   - 快速開發

3. **Electron**:
   - 開發速度快
   - 社區資源豐富
   - 但體積較大

### Windows 打印機 API

使用 Windows Print Spooler API 或 PowerShell 命令獲取打印機列表：

```powershell
Get-Printer | Select-Object Name, DriverName, PortName, Default
```

### Kpay 集成

需要參考 Kpay SDK 文檔實現：
- TCP/IP 通信
- 交易協議處理
- 收據打印格式

## 安全考慮

1. **只監聽 localhost** - 不暴露到網絡
2. **CORS 設置** - 只允許 vwork 域名訪問
3. **請求驗證** - 可選添加 token 驗證
4. **日誌記錄** - 記錄所有交易操作

## 版本歷史

- **v1.0.0** (計劃中)
  - 基本打印機列表獲取
  - 卡機連接和支付
  - 熱敏打印機支援
