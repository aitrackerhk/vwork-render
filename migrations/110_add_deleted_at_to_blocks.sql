-- 為 blocks 表添加 deleted_at 列（軟刪除支持）
ALTER TABLE blocks ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP WITH TIME ZONE;

-- 為 deleted_at 創建索引
CREATE INDEX IF NOT EXISTS idx_blocks_deleted_at ON blocks(deleted_at);

