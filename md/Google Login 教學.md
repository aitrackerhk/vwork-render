# Google OAuth 登錄配置指南

## 403 Forbidden 錯誤解決方案

如果您遇到 `403 (Forbidden)` 錯誤，通常是因為當前域名未在 Google Cloud Console 中授權。

### 解決步驟

1. **訪問 Google Cloud Console**
   - 前往：https://console.cloud.google.com/apis/credentials
   - 選擇您的項目

2. **編輯 OAuth 2.0 客戶端 ID**
   - 找到您的 OAuth 2.0 客戶端 ID
   - 點擊編輯（鉛筆圖標）

3. **添加授權的 JavaScript 來源**
   在「授權的 JavaScript 來源」部分，添加以下 URL：

   **生產環境：**
   - `https://vworkai.com`
   - `https://www.vworkai.com`

   **開發環境（本地測試）：**
   - `http://localhost:3001`
   - `http://127.0.0.1:3001`

   **測試環境：**
   - 如果使用其他域名，請添加完整的域名（例如：`https://test.vworkai.com`）

4. **保存更改**
   - 點擊「保存」
   - 等待幾分鐘讓更改生效（通常 1-5 分鐘）

### 重要提示

- ✅ 不要包含尾部斜線（`/`）
- ✅ 必須包含協議（`http://` 或 `https://`）
- ✅ 域名必須完全匹配（包括端口號，如果是非標準端口）
- ✅ 更改可能需要幾分鐘才能生效

### 驗證配置

1. 刷新您的應用頁面
2. 點擊 Google 登錄按鈕
3. 應該會看到 Google 登錄彈窗，而不是 403 錯誤

### 當前配置的 Client ID

```
150732813516-90v6kgagrtcoagdrppr7o0beo69b7f96.apps.googleusercontent.com
```

### 常見問題

**Q: 我添加了 localhost，但還是出現 403 錯誤？**
A: 確保：
- 使用的是 `http://localhost:3001`（包含端口號）
- 或者嘗試 `http://127.0.0.1:3001`
- 清除瀏覽器緩存並刷新頁面

**Q: 生產環境需要配置哪些域名？**
A: 配置所有可能訪問您應用的域名：
- 主域名（例如：`https://vworkai.com`）
- www 子域名（例如：`https://www.vworkai.com`）
- 任何子域名（例如：`https://app.vworkai.com`）

**Q: 為什麼需要配置多個域名？**
A: Google OAuth 會檢查請求的來源（origin），必須完全匹配配置的域名。如果您有多個入口點（www、非 www、子域名等），都需要分別配置。

