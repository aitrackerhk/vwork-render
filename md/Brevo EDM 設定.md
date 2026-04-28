# 🚀 立即設置 Brevo SMTP

## 快速設置步驟

### 方法 1：使用 PowerShell 腳本（推薦）

運行以下命令：

```powershell
cd C:\Users\tednv\nir\nwork
powershell -ExecutionPolicy Bypass -File .\scripts\setup_brevo_smtp.ps1
```

### 方法 2：手動設置

1. **創建或編輯 `.env` 文件**

   在項目根目錄 `C:\Users\tednv\nir\nwork\.env` 中，找到或添加以下配置：

   ```env
   # --- SMTP (Brevo) ---
   SMTP_HOST=smtp-relay.brevo.com
   SMTP_PORT=587
   SMTP_USE_STARTTLS=true
   SMTP_INSECURE_SKIP_VERIFY_TLS=false

   # Brevo SMTP credentials
   SMTP_USER=9f0077001@smtp-brevo.com
   SMTP_PASSWORD=xsmtpsib-0a2cfb1d137b629bd7a568331b7c9bd8ce1beca2082a326e94e1e56dd1a4309b-4DmELMju6vz3ZF0F

   # Sender information
   SMTP_FROM_EMAIL=no-reply@mail.vworkai.com
   SMTP_FROM_NAME=vWork
   ```

2. **如果沒有 `.env` 文件**

   從 `env.example` 複製：
   ```powershell
   cd C:\Users\tednv\nir\nwork
   Copy-Item env.example .env
   ```

   然後編輯 `.env` 文件，將上述 SMTP 配置替換空白的 SMTP 配置部分。

---

## ✅ 驗證配置

### 1. 檢查 `.env` 文件

確認以下配置正確：
- ✅ `SMTP_HOST=smtp-relay.brevo.com`
- ✅ `SMTP_PORT=587`
- ✅ `SMTP_USER=9f0077001@smtp-brevo.com`
- ✅ `SMTP_PASSWORD=xsmtpsib-0a2cfb1d137b629bd7a568331b7c9bd8ce1beca2082a326e94e1e56dd1a4309b-4DmELMju6vz3ZF0F`
- ✅ `SMTP_FROM_EMAIL=no-reply@mail.vworkai.com`
- ✅ `SMTP_FROM_NAME=vWork`

### 2. 重啟 Email Worker

```powershell
cd C:\Users\tednv\nir\nwork
go run cmd/email_worker/main.go
```

### 3. 測試發送

在應用中觸發一封測試郵件（例如：註冊新用戶），然後檢查 email worker 的輸出日誌。

---

## ⚠️ 重要提醒

1. **域名驗證**：在 Brevo 控制台中驗證域名 `mail.vworkai.com` 或 `vworkai.com`
2. **免費方案限制**：每天 300 封郵件，只能發送給已驗證的郵件地址
3. **安全性**：確保 `.env` 文件已添加到 `.gitignore`

---

## 🔍 故障排除

如果 email worker 啟動時顯示錯誤：

1. **檢查配置**：確認 `.env` 文件中的配置完全正確
2. **檢查日誌**：查看 email worker 的錯誤訊息
3. **測試連接**：確認網絡可以連接到 `smtp-relay.brevo.com:587`

---

**配置完成後，email worker 就可以使用 Brevo 發送郵件了！** 🎉













