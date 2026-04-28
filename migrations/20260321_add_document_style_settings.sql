-- 文件樣式設定：為 document_settings 表新增 logo / 字型大小等樣式欄位
-- document_type = 'style' 代表全域文件樣式設定（所有文件類型共用）

ALTER TABLE document_settings ADD COLUMN IF NOT EXISTS logo_url VARCHAR(500);
ALTER TABLE document_settings ADD COLUMN IF NOT EXISTS logo_width DECIMAL(8,2) DEFAULT 0;
ALTER TABLE document_settings ADD COLUMN IF NOT EXISTS logo_height DECIMAL(8,2) DEFAULT 0;
ALTER TABLE document_settings ADD COLUMN IF NOT EXISTS title_font_size DECIMAL(5,2) DEFAULT 0;
ALTER TABLE document_settings ADD COLUMN IF NOT EXISTS body_font_size DECIMAL(5,2) DEFAULT 0;
ALTER TABLE document_settings ADD COLUMN IF NOT EXISTS notes_font_size DECIMAL(5,2) DEFAULT 0;
