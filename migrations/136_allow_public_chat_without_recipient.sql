-- 允許 public_chat 類型的消息沒有收件人（to_user_id 和 to_customer_id 都可以為 NULL）
-- 更新 chk_message_recipient 約束，添加 public_chat 例外

-- 先刪除現有約束
ALTER TABLE messages DROP CONSTRAINT IF EXISTS chk_message_recipient;

-- 重新添加約束：允許 ai_chat 和 public_chat 沒有收件人
ALTER TABLE messages ADD CONSTRAINT chk_message_recipient 
    CHECK (
        (message_type = 'ai_chat') OR 
        (message_type = 'public_chat') OR 
        (to_user_id IS NOT NULL) OR 
        (to_customer_id IS NOT NULL)
    );

