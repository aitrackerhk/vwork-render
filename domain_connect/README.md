## Domain Connect（Cloudflare）- vWork / vBuilder

此目錄用於準備 **Domain Connect 模板**，讓「客戶的 DNS 在 Cloudflare」時，可以用 **一鍵套用模板**自動建立 DNS 記錄。

### 你現在的產品目標（最簡單）
- 只支援 `www.customer.com`
- 自動建立：`www` → CNAME → `cname.vworkai.com`
- SSL 由 Cloudflare for SaaS/SSL for SaaS 處理（不需要 `_acme-challenge`）

### 重要前提（不符合就無法全自動）
- 客戶網域的 DNS 必須在 Cloudflare（NS 指到 Cloudflare）
- 模板需要提交到 Domain Connect templates repo，並請 Cloudflare 上線到他們的 Domain Connect 系統

### 檔案
- `templates/vwork/www-cname.json`：模板（只建 `www` 的 CNAME）
- `email_to_cloudflare.md`：寄給 Cloudflare 的上線申請信範本
- `logo.svg`：Cloudflare Domain Connect UI 會顯示的 logo（可換成正式品牌）

### 下一步（上線）
1) 把 template 依 Domain Connect 官方格式確認無誤（不同 DNS provider 可能有額外要求）
2) 提交 PR 到 Domain Connect templates repo（把你的 `providerId/serviceId` 放進對應路徑）
3) 依 Cloudflare 文件寄信到 `domain-connect@cloudflare.com` 申請上線


