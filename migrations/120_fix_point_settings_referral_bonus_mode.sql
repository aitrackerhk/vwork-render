-- 120_fix_point_settings_referral_bonus_mode.sql
-- 确保 point_settings 表有 referral_bonus_mode 和 referral_bonus_value 列

ALTER TABLE point_settings
  ADD COLUMN IF NOT EXISTS referral_bonus_mode varchar(20) DEFAULT 'fixed';

ALTER TABLE point_settings
  ADD COLUMN IF NOT EXISTS referral_bonus_value decimal(10,2) DEFAULT 0;

-- 如果列已存在但为 NULL，设置默认值
UPDATE point_settings
SET referral_bonus_mode = COALESCE(referral_bonus_mode, 'fixed')
WHERE referral_bonus_mode IS NULL;

UPDATE point_settings
SET referral_bonus_value = COALESCE(referral_bonus_value, 0)
WHERE referral_bonus_value IS NULL;

-- 将旧的 referral_bonus_points 回填到 referral_bonus_value（定額模式）
UPDATE point_settings
SET referral_bonus_mode = 'fixed',
    referral_bonus_value = COALESCE(referral_bonus_value, 0) + COALESCE(referral_bonus_points, 0)
WHERE (referral_bonus_value IS NULL OR referral_bonus_value = 0)
  AND COALESCE(referral_bonus_points, 0) > 0
  AND (referral_bonus_mode IS NULL OR referral_bonus_mode = 'fixed');

