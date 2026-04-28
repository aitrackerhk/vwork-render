-- 179: Add enable_points_adjustment_on_edit column to point_settings
-- 編輯訂單/服務單時補發積分差值開關

ALTER TABLE point_settings
ADD COLUMN IF NOT EXISTS enable_points_adjustment_on_edit BOOLEAN DEFAULT TRUE;

COMMENT ON COLUMN point_settings.enable_points_adjustment_on_edit IS '編輯訂單/服務單時，如果積分增加，是否補發差值到客戶積分';
