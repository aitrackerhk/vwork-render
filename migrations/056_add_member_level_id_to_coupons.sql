-- 為 coupons 表添加 member_level_id 欄位（如果不存在）
ALTER TABLE coupons
    ADD COLUMN IF NOT EXISTS member_level_id UUID REFERENCES member_levels(id) ON DELETE SET NULL;

-- 添加索引以提高查詢性能
CREATE INDEX IF NOT EXISTS idx_coupons_member_level_id ON coupons(member_level_id);

-- 添加註釋
COMMENT ON COLUMN coupons.member_level_id IS '限制特定會員等級使用';

