-- 建立電話區號表並預置常用全球區號，預設 +852
CREATE TABLE IF NOT EXISTS phone_country_codes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code VARCHAR(10) NOT NULL UNIQUE,
    name VARCHAR(100) NOT NULL,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- 預置資料（如已存在則跳過）
INSERT INTO phone_country_codes (code, name, is_default)
SELECT v.code, v.name, v.is_default
FROM (VALUES
    ('+852', 'Hong Kong', TRUE),
    ('+853', 'Macau', FALSE),
    ('+86',  'China', FALSE),
    ('+886', 'Taiwan', FALSE),
    ('+81',  'Japan', FALSE),
    ('+82',  'South Korea', FALSE),
    ('+1',   'United States/Canada', FALSE),
    ('+44',  'United Kingdom', FALSE),
    ('+61',  'Australia', FALSE),
    ('+65',  'Singapore', FALSE),
    ('+60',  'Malaysia', FALSE),
    ('+62',  'Indonesia', FALSE),
    ('+63',  'Philippines', FALSE),
    ('+66',  'Thailand', FALSE),
    ('+84',  'Vietnam', FALSE),
    ('+971', 'United Arab Emirates', FALSE),
    ('+91',  'India', FALSE),
    ('+33',  'France', FALSE),
    ('+49',  'Germany', FALSE),
    ('+39',  'Italy', FALSE),
    ('+34',  'Spain', FALSE),
    ('+41',  'Switzerland', FALSE),
    ('+7',   'Russia', FALSE),
    ('+55',  'Brazil', FALSE),
    ('+52',  'Mexico', FALSE),
    ('+54',  'Argentina', FALSE),
    ('+64',  'New Zealand', FALSE),
    ('+27',  'South Africa', FALSE)
) AS v(code, name, is_default)
WHERE NOT EXISTS (
    SELECT 1 FROM phone_country_codes p WHERE p.code = v.code
);

-- 僅允許一筆預設，若多筆預設則保留最早一筆
WITH defaults AS (
    SELECT id FROM phone_country_codes WHERE is_default = TRUE ORDER BY created_at ASC
), keep AS (
    SELECT id FROM defaults LIMIT 1
)
UPDATE phone_country_codes
SET is_default = FALSE
WHERE is_default = TRUE AND id NOT IN (SELECT id FROM keep);


