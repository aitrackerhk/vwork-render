-- Add order_notifications_enabled to notification_settings table
-- 添加訂單提示資訊開關

ALTER TABLE notification_settings 
ADD COLUMN IF NOT EXISTS order_notifications_enabled BOOLEAN DEFAULT true;

COMMENT ON COLUMN notification_settings.order_notifications_enabled IS '訂單提示資訊開關';

