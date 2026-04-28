# vWork 技術記錄

## 後端
- **語言**: Go 1.21+
- **Web Framework**: Fiber
  - Express 風格，對 Node.js 開發者友好
  - 高性能、輕量級
  - 中間件豐富
- **ORM**: 
  - **建議**: GORM (功能完整、文檔豐富)
  - **備選**: sqlx (更接近原生 SQL)
- **數據庫**: PostgreSQL 14+
- **模板引擎**: 
  - Fiber 內建 HTML 模板支持
  - 或使用 Go 標準庫 `html/template`

## 前端
- **技術**: Bootstrap 5 + HTML + JavaScript (Vanilla JS)
- **不使用**: React, Vue, Angular 等框架
- **理由**: 
  - 簡單直接，無需構建工具
  - 快速開發，易於維護
  - 適合中小型 ERP 系統
  - 減少複雜度和依賴

## UI 框架
- **Bootstrap 5** (已選定)
  - 響應式設計，支持手機端
  - 組件豐富（表格、表單、模態框等）
  - 文檔完善
  - 可選用 Bootstrap Icons 或 Font Awesome 圖標

## 前端架構
- **靜態資源**: `web/static/` 目錄
  - `css/` - Bootstrap CSS 和自定義樣式
  - `js/` - JavaScript 文件
  - `images/` - 圖片資源
- **模板文件**: `web/templates/` 目錄
  - Go HTML 模板
  - 可重用組件（header, footer, sidebar 等）

## 認證與安全
- JWT (JSON Web Tokens)
- bcrypt (密碼加密)
- CORS 中間件
- CSRF 保護

## API 設計
- RESTful API
- JSON 響應格式
- 統一錯誤處理

## 部署
- **容器化**: Docker
- **CI/CD**: GitHub Actions 或 GitLab CI
- **雲服務**: 
  - AWS / Azure / GCP
  - 或 VPS (DigitalOcean, Linode 等)

## 開發工具
- **API 文檔**: Swagger/OpenAPI (可選)
- **測試**: Go testing
- **代碼格式化**: gofmt
- **前端工具**: 無需構建工具，直接使用瀏覽器

## 項目結構（前端部分）
```
web/
├── static/
│   ├── css/
│   │   ├── bootstrap.min.css
│   │   └── custom.css
│   ├── js/
│   │   ├── bootstrap.bundle.min.js
│   │   └── app.js
│   └── images/
└── templates/
    ├── layouts/
    │   └── base.html
    ├── pages/
    │   ├── dashboard.html
    │   ├── customers.html
    │   └── ...
    └── components/
        ├── header.html
        ├── sidebar.html
        └── footer.html
```
