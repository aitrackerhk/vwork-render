-- 079_add_referral_bonus_mode_to_point_settings.sql
-- 介紹人獎勵積分：支援「百分比 / 定額」兩種模式

ALTER TABLE point_settings
  ADD COLUMN IF NOT EXISTS referral_bonus_mode varchar(20) NOT NULL DEFAULT 'fixed',
  ADD COLUMN IF NOT EXISTS referral_bonus_value decimal(10,2) NOT NULL DEFAULT 0;

-- 將舊的 referral_bonus_points 回填到 referral_bonus_value（定額模式）
UPDATE point_settings
SET referral_bonus_mode = 'fixed',
    referral_bonus_value = COALESCE(referral_bonus_value, 0) + COALESCE(referral_bonus_points, 0)
WHERE (referral_bonus_value IS NULL OR referral_bonus_value = 0)
  AND COALESCE(referral_bonus_points, 0) > 0;


