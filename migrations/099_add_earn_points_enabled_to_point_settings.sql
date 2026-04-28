-- 積分設置：是否啟用「消費獲得積分」

ALTER TABLE point_settings
ADD COLUMN IF NOT EXISTS earn_points_enabled BOOLEAN NOT NULL DEFAULT TRUE;


