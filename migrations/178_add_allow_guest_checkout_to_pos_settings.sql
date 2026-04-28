-- 178: Add allow_guest_checkout to pos_settings table
-- Allow guest checkout option for POS

ALTER TABLE pos_settings 
ADD COLUMN IF NOT EXISTS allow_guest_checkout BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN pos_settings.allow_guest_checkout IS '允許訪客結帳：啟用後 POS 結帳時若未選擇客戶會提示以訪客名義結帳';
