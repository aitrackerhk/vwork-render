-- 創建國家地區表
CREATE TABLE IF NOT EXISTS country_regions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    country_code VARCHAR(10) NOT NULL,
    region_code VARCHAR(50) NOT NULL,
    region_name VARCHAR(255) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(country_code, region_code)
);

-- 創建索引
CREATE INDEX IF NOT EXISTS idx_country_regions_country_code ON country_regions(country_code);
CREATE INDEX IF NOT EXISTS idx_country_regions_region_code ON country_regions(region_code);

-- 添加註釋
COMMENT ON TABLE country_regions IS '國家地區表（從 GeoNames API 獲取）';
COMMENT ON COLUMN country_regions.country_code IS '國家代碼（ISO 3166-1 alpha-2）';
COMMENT ON COLUMN country_regions.region_code IS '地區代碼';
COMMENT ON COLUMN country_regions.region_name IS '地區名稱';

