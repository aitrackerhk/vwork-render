-- 支持消息发送给客户
-- 修改 messages 表，添加 to_customer_id 字段，并将 to_user_id 改为可选

-- 步骤 1: 添加 to_customer_id 字段（如果不存在）
ALTER TABLE messages ADD COLUMN IF NOT EXISTS to_customer_id UUID REFERENCES customers(id);

-- 步骤 2: 处理现有数据 - 对于 AI 消息（message_type = 'ai_chat'），允许 to_user_id 和 to_customer_id 都为 NULL
-- 对于其他消息，如果 to_user_id 为 NULL，我们需要确保至少有一个不为 NULL
-- 但为了安全，我们先不添加约束，让现有数据通过

-- 步骤 3: 将 to_user_id 改为可选（移除 NOT NULL 约束）
-- 注意：PostgreSQL 的 NOT NULL 是列属性，不是约束
-- 使用 DO 块来安全地移除 NOT NULL，避免如果列已经是可选的时报错
DO $$
BEGIN
    -- 尝试移除 NOT NULL，如果已经是可选的则忽略错误
    ALTER TABLE messages ALTER COLUMN to_user_id DROP NOT NULL;
EXCEPTION
    WHEN OTHERS THEN
        -- 如果列已经是可选的或不存在，忽略错误
        NULL;
END $$;

-- 步骤 4: 添加索引以提高查询性能
CREATE INDEX IF NOT EXISTS idx_messages_to_customer_id ON messages(to_customer_id);
CREATE INDEX IF NOT EXISTS idx_messages_to_user_id ON messages(to_user_id);

-- 步骤 5: 添加检查约束：to_user_id 和 to_customer_id 至少有一个不为 NULL
-- 但对于 AI 消息（message_type = 'ai_chat'），允许两者都为 NULL
-- 先删除约束（如果存在），然后重新添加
DO $$
BEGIN
    -- 尝试删除约束（如果存在）
    ALTER TABLE messages DROP CONSTRAINT IF EXISTS chk_message_recipient;
EXCEPTION
    WHEN OTHERS THEN
        NULL;
END $$;

-- 添加检查约束：对于非 AI 消息，to_user_id 和 to_customer_id 至少有一个不为 NULL
-- 对于 AI 消息，允许两者都为 NULL
ALTER TABLE messages ADD CONSTRAINT chk_message_recipient 
    CHECK (
        (message_type = 'ai_chat') OR 
        (to_user_id IS NOT NULL) OR 
        (to_customer_id IS NOT NULL)
    );

