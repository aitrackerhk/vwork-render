-- 為物流公司添加配送連接類型
-- integration_type: none, sfexpress, lalamove
-- 用於標識該物流公司是否與第三方配送 API 整合

ALTER TABLE logistics_companies 
ADD COLUMN IF NOT EXISTS integration_type VARCHAR(50) DEFAULT 'none';

-- 添加註解
COMMENT ON COLUMN logistics_companies.integration_type IS '配送連接類型：none（無整合）、sfexpress（順豐速遞）、lalamove（啦啦快送）';
