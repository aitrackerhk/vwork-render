# WhatsApp Business API 設置指南

本指南說明如何讓一個電話號碼可以使用 WhatsApp 官方 API。

## 📱 重要提示：與 WhatsApp Business App 共存

**好消息**：您可以使用同一個電話號碼同時運行 WhatsApp Business App 和 API！

### 推薦流程：先使用 App，後添加 API

✅ **完全可以先安裝 WhatsApp Business App 使用**，之後需要時再添加 API 功能：

1. **第一步**：現在就可以安裝 WhatsApp Business App
   - 下載並安裝 WhatsApp Business App
   - 使用您的業務電話號碼註冊
   - 開始正常使用（所有功能都可用）

2. **第二步**：當需要 API 功能時
   - 按照本指南設置 WhatsApp Business API
   - 在添加電話號碼時選擇「與 WhatsApp Business App 共存」
   - 啟用後，App 和 API 可以同時使用

### 共存模式的優勢

- ✅ 可以在手機上繼續使用 WhatsApp Business App 進行個人聊天
- ✅ 同時通過 API 發送自動化消息和集成到系統中
- ✅ 消息和聯繫人會實時同步
- ✅ 過去 6 個月的聊天記錄會被同步
- ⚠️ 某些功能（如群組、視頻通話）在共存模式下可能受限
- 💰 App 發送的消息免費，API 發送的消息按 Meta 定價收費

詳細說明請見下方「重要注意事項」部分。

## 方式一：使用 Meta WhatsApp Cloud API（推薦）

Meta 提供官方的 WhatsApp Cloud API，可以直接使用，無需通過第三方 BSP。

### 步驟 1: 創建 Meta Business 帳戶

1. 訪問 [Meta for Developers](https://developers.facebook.com/)
2. 創建或登錄您的 Facebook 帳戶
3. 前往 [Meta Business Suite](https://business.facebook.com/) 創建業務帳戶
4. 完成業務驗證（可能需要提供業務文件）

### 步驟 2: 創建 WhatsApp 應用

1. 在 [Meta for Developers](https://developers.facebook.com/apps/) 創建新應用
2. 選擇應用類型：**Business**
3. 添加 **WhatsApp** 產品到您的應用
4. 完成應用設置

### 步驟 3: 獲取訪問令牌和電話號碼 ID

1. 在 WhatsApp 產品設置中，您會看到：
   - **臨時訪問令牌** (Temporary Access Token) - 用於測試
   - **應用 ID** (App ID)
   - **應用密鑰** (App Secret)

2. 添加電話號碼：
   - 點擊「添加電話號碼」
   - 輸入您要使用的電話號碼（需要能夠接收驗證碼）
   - 選擇驗證方式（短信或語音）
   - 輸入收到的驗證碼
   - 完成後會獲得 **電話號碼 ID** (Phone Number ID)

### 步驟 4: 配置 Webhook

1. 在 WhatsApp 產品設置中，找到「配置」→「Webhook」
2. 設置回調 URL：`https://yourdomain.com/api/whatsapp/webhook`
3. 設置驗證令牌（自定義一個安全字符串）
4. 訂閱以下事件：
   - `messages` - 接收消息
   - `message_status` - 消息狀態更新

### 步驟 5: 獲取永久訪問令牌

臨時令牌只有 24 小時有效期，需要獲取永久令牌：

1. 在 Meta Business Suite 中創建系統用戶
2. 為系統用戶分配 WhatsApp Business 權限
3. 生成永久訪問令牌

### 步驟 6: 配置環境變量

在您的 `.env` 文件中添加以下配置：

```env
# WhatsApp Business API 配置
WHATSAPP_API_TOKEN=your_permanent_access_token
WHATSAPP_PHONE_NUMBER_ID=your_phone_number_id
WHATSAPP_APP_ID=your_app_id
WHATSAPP_APP_SECRET=your_app_secret
WHATSAPP_VERIFY_TOKEN=your_webhook_verify_token
WHATSAPP_API_VERSION=v21.0
WHATSAPP_BUSINESS_ACCOUNT_ID=your_business_account_id
```

### 步驟 7: 消息模板審批

在發送主動消息（非回覆）之前，需要創建並提交消息模板：

1. 在 WhatsApp Manager 中創建消息模板
2. 提交審批（通常需要 24-48 小時）
3. 審批通過後才能使用該模板發送消息

## 方式二：通過 BSP（Business Solution Provider）

如果不想直接使用 Meta API，可以通過認證的 BSP：

### 常見 BSP 提供商

1. **YCloud** - https://www.ycloud.com/
2. **WOZTELL** - https://www.woztell.com/
3. **Twilio** - https://www.twilio.com/whatsapp
4. **MessageBird** - https://www.messagebird.com/

### BSP 設置流程

1. 註冊 BSP 帳戶
2. 創建 WhatsApp Business API 帳戶
3. 驗證電話號碼
4. 獲取 API Key 和端點
5. 配置到系統中

## API 端點說明

### Meta WhatsApp Cloud API 端點

- **發送消息**: `POST https://graph.facebook.com/v21.0/{phone-number-id}/messages`
- **上傳媒體**: `POST https://graph.facebook.com/v21.0/{phone-number-id}/media`
- **Webhook 驗證**: `GET /api/whatsapp/webhook` (驗證請求)
- **接收消息**: `POST /api/whatsapp/webhook` (接收消息)

### 請求範例

#### 發送文本消息

```bash
curl -X POST "https://graph.facebook.com/v21.0/{PHONE_NUMBER_ID}/messages" \
  -H "Authorization: Bearer {ACCESS_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "messaging_product": "whatsapp",
    "to": "85212345678",
    "type": "text",
    "text": {
      "body": "Hello, this is a test message"
    }
  }'
```

#### 發送模板消息

```bash
curl -X POST "https://graph.facebook.com/v21.0/{PHONE_NUMBER_ID}/messages" \
  -H "Authorization: Bearer {ACCESS_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "messaging_product": "whatsapp",
    "to": "85212345678",
    "type": "template",
    "template": {
      "name": "hello_world",
      "language": {
        "code": "en"
      }
    }
  }'
```

## 重要注意事項

1. **24 小時會話窗口**：
   - 客戶發送消息後，您有 24 小時可以免費回覆
   - 24 小時後需要使用已審批的模板發送

2. **消息類型限制**：
   - 在 24 小時窗口內：可以發送任何類型的消息
   - 在 24 小時窗口外：只能發送已審批的模板消息

3. **費用**：
   - 對話費用根據國家/地區不同
   - 查看 [WhatsApp 定價](https://developers.facebook.com/docs/whatsapp/pricing)

4. **電話號碼要求**：
   - 必須是真實的電話號碼
   - 不能是已註冊 WhatsApp 個人帳戶的號碼
   - 建議使用專用的業務號碼

5. **測試號碼**：
   - Meta 提供測試號碼，但只能發送給已註冊的測試號碼
   - 生產環境需要使用真實的業務號碼

6. **與 WhatsApp Business App 共存**：
   - ✅ **可以共存**：同一個電話號碼可以同時使用 WhatsApp Business App 和 API
   - **實時同步**：聊天記錄和聯繫人會在 App 和 API 之間同步
   - **保留歷史**：啟用共存後，過去 6 個月的聊天記錄會被同步
   - **功能限制**：共存模式下，以下功能可能無法使用：
     - 群組聊天
     - 消失的消息
     - 查看一次的消息
     - 實時位置分享
     - 視頻和語音通話
   - **費用**：
     - 通過 WhatsApp Business App 發送的消息：**免費**
     - 通過 API 發送的消息：按 Meta 定價標準收費
   - **如何啟用共存**：
     - 如果使用 Meta Cloud API：在添加電話號碼時選擇「與 WhatsApp Business App 共存」選項
     - 如果使用 BSP：聯繫服務提供商協助啟用共存功能
     - 確保 WhatsApp Business App 已更新至最新版本

## 代碼集成說明

### 已實現的功能

1. **WhatsApp API 客戶端** (`nwork/internal/whatsapp/client.go`)
   - 發送文本消息
   - 發送模板消息
   - 上傳媒體文件

2. **Webhook 處理器** (`nwork/internal/whatsapp/webhook.go`)
   - Webhook 驗證
   - 接收消息和狀態更新
   - 解析 Webhook 事件

3. **Handler 集成** (`nwork/internal/handlers/whatsapp.go`)
   - Webhook 端點處理
   - 推廣消息發送
   - 消息處理邏輯

4. **推廣功能集成** (`nwork/internal/handlers/support.go`)
   - 已更新 `SendPromotion` 函數以支持 WhatsApp 發送

### 使用方式

#### 1. 發送文本消息

```go
import "nwork/internal/whatsapp"

client := whatsapp.NewClient(cfg)
err := client.SendTextMessage("85212345678", "Hello, this is a test message")
```

#### 2. 發送模板消息

```go
parameters := []map[string]string{
    {"type": "text", "text": "John"},
    {"type": "text", "text": "12345"},
}
err := client.SendTemplateMessage("85212345678", "order_confirmation", "en", parameters)
```

#### 3. 在推廣中使用

推廣功能已自動集成 WhatsApp API。當創建推廣時選擇 "whatsapp" 類型，系統會自動使用 WhatsApp API 發送消息。

### Webhook 端點

- **驗證**: `GET /api/whatsapp/webhook`
- **接收消息**: `POST /api/whatsapp/webhook`

在 Meta 開發者控制台中配置 Webhook URL 為：
```
https://yourdomain.com/api/whatsapp/webhook
```

## 下一步

完成上述設置後，請確保：
1. ✅ Webhook 端點已正確配置到 Meta 開發者控制台
2. ✅ 環境變量已正確設置
3. ✅ 消息模板已創建並通過審批（用於 24 小時窗口外的消息）
4. ✅ 測試消息發送功能
5. ✅ 測試 Webhook 接收功能
6. ⚠️ 根據業務需求完善 `handleIncomingWhatsAppMessage` 和 `handleWhatsAppStatusUpdate` 函數

## 參考資源

- [WhatsApp Business API 文檔](https://developers.facebook.com/docs/whatsapp)
- [WhatsApp Cloud API 快速開始](https://developers.facebook.com/docs/whatsapp/cloud-api/get-started)
- [消息模板指南](https://developers.facebook.com/docs/whatsapp/message-templates)
- [Webhook 設置指南](https://developers.facebook.com/docs/whatsapp/cloud-api/webhooks)

