-- Create notification_settings table
-- 系統提示設定表：控制各類通知是否啟用

CREATE TABLE IF NOT EXISTS notification_settings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    -- 各類通知開關
    attendance_notifications_enabled BOOLEAN DEFAULT true,  -- 打卡提示資訊
    service_order_notifications_enabled BOOLEAN DEFAULT true,  -- 服務單提示資訊
    appointment_notifications_enabled BOOLEAN DEFAULT true,  -- 預約提示資訊
    project_due_notifications_enabled BOOLEAN DEFAULT true,  -- 項目/task 到期提示資訊
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id)
);

CREATE INDEX IF NOT EXISTS idx_notification_settings_tenant_id ON notification_settings(tenant_id);

COMMENT ON TABLE notification_settings IS '系統提示設定表：控制各類通知是否啟用';
COMMENT ON COLUMN notification_settings.attendance_notifications_enabled IS '打卡提示資訊開關（需 HR 模組）';
COMMENT ON COLUMN notification_settings.service_order_notifications_enabled IS '服務單提示資訊開關（需服務單模組）';
COMMENT ON COLUMN notification_settings.appointment_notifications_enabled IS '預約提示資訊開關（需服務單模組）';
COMMENT ON COLUMN notification_settings.project_due_notifications_enabled IS '項目/task 到期提示資訊開關（需項目管理模組）';

