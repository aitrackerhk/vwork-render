-- 插入默認貨幣 HKD 和 USD

-- 插入港幣 (HKD) - 作為基礎貨幣，匯率為 1.0
INSERT INTO currencies (code, name, symbol, exchange_rate, status, created_at, updated_at)
SELECT 'HKD', '港幣', '$', 1.0, 'active', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
WHERE NOT EXISTS (SELECT 1 FROM currencies WHERE code = 'HKD');

-- 插入美元 (USD)
INSERT INTO currencies (code, name, symbol, exchange_rate, status, created_at, updated_at)
SELECT 'USD', '美元', '$', 1.0, 'active', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
WHERE NOT EXISTS (SELECT 1 FROM currencies WHERE code = 'USD');

