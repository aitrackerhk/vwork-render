-- notification_alerts：加入 dedupe_key 防止 cron 重覆生成
ALTER TABLE notification_alerts
    ADD COLUMN IF NOT EXISTS dedupe_key VARCHAR(255);

CREATE UNIQUE INDEX IF NOT EXISTS idx_notification_alerts_dedupe_key_unique
    ON notification_alerts(dedupe_key)
    WHERE dedupe_key IS NOT NULL AND dedupe_key <> '';


