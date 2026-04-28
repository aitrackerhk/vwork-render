# vWork 架構設計

## 多租戶架構

### 策略：共享數據庫，租戶隔離
- 所有表包含 `tenant_id` 字段
- 通過中間件自動過濾租戶數據
- 使用 PostgreSQL 的 Row Level Security (RLS) 可選

## 動態字段設計

### 實現方式：JSONB
- 每個表都有 `extra_fields JSONB` 字段
- 允許存儲任意結構的額外字段
- 支持索引和查詢（使用 GIN 索引）

### 使用示例
```sql
-- 添加動態字段
UPDATE customers 
SET extra_fields = extra_fields || '{"custom_field_1": "value", "custom_field_2": 123}'::jsonb
WHERE id = 'xxx';

-- 查詢動態字段
SELECT * FROM customers 
WHERE extra_fields->>'custom_field_1' = 'value';
```

### API 設計
- GET /api/v1/customers/:id/fields - 獲取所有動態字段
- POST /api/v1/customers/:id/fields - 添加/更新動態字段
- DELETE /api/v1/customers/:id/fields/:field_name - 刪除動態字段

## 核心模塊

1. **認證模塊**
   - 用戶註冊/登錄
   - JWT Token 管理
   - 密碼重置

2. **租戶模塊**
   - 租戶註冊
   - 訂閱管理
   - 計劃升級/降級

3. **ERP 核心模塊**
   - 客戶管理
   - 產品管理
   - 訂單管理
   - 庫存管理
   - 發票管理
   - 支付管理

4. **動態字段模塊**
   - 字段定義管理
   - 字段值 CRUD
   - 字段驗證

## API 設計原則

- RESTful 風格
- 版本控制：/api/v1/
- 統一響應格式
- 錯誤處理標準化
- 分頁支持

## 安全考慮

- 所有 API 需要認證（除公開端點）
- 租戶數據隔離（中間件層）
- SQL 注入防護（使用參數化查詢）
- XSS 防護
- CSRF 防護

