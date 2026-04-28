-- 為客戶增加電話國碼與生日
ALTER TABLE customers
    ADD COLUMN IF NOT EXISTS phone_country_code VARCHAR(10),
    ADD COLUMN IF NOT EXISTS birth_date DATE;

-- 簡單索引（若後續要按生日查詢可用）
CREATE INDEX IF NOT EXISTS idx_customers_birth_date ON customers(birth_date);

