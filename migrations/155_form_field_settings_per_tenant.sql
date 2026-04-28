-- 155_form_field_settings_per_tenant.sql
-- 將表單欄位設定從「每用戶」改為「每租戶」
-- 欄位設定應該是租戶級別的，所有用戶共享同一套設定

-- 先移除現有的唯一約束和索引
DROP INDEX IF EXISTS idx_user_form_field_settings_tenant_user;
DROP INDEX IF EXISTS idx_user_form_field_settings_page;

-- 移除舊的唯一約束（如果存在）
ALTER TABLE user_form_field_settings 
DROP CONSTRAINT IF EXISTS user_form_field_settings_tenant_id_user_id_page_name_key;
ALTER TABLE user_form_field_settings
DROP CONSTRAINT IF EXISTS user_form_field_settings_tenant_user_page_uk;

-- 刪除重複的記錄，只保留每個 tenant_id + page_name 的最新一條
DELETE FROM user_form_field_settings a
USING user_form_field_settings b
WHERE a.id < b.id 
  AND a.tenant_id = b.tenant_id 
  AND a.page_name = b.page_name;

-- 移除 user_id 欄位
ALTER TABLE user_form_field_settings 
DROP COLUMN IF EXISTS user_id;

-- 添加新的唯一約束：每個租戶每個頁面只有一條設定記錄
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'user_form_field_settings_tenant_page_unique'
  ) THEN
    ALTER TABLE user_form_field_settings
    ADD CONSTRAINT user_form_field_settings_tenant_page_unique UNIQUE(tenant_id, page_name);
  END IF;
END $$;

-- 創建新的索引
CREATE INDEX IF NOT EXISTS idx_user_form_field_settings_tenant ON user_form_field_settings(tenant_id);
CREATE INDEX IF NOT EXISTS idx_user_form_field_settings_tenant_page ON user_form_field_settings(tenant_id, page_name);

-- 更新註釋
COMMENT ON TABLE user_form_field_settings IS '租戶表單欄位設定表：存儲租戶級別的表單欄位顯示/隱藏、排序和額外欄位';
