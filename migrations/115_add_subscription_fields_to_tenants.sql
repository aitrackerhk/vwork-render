-- 添加訂閱相關字段到 tenants 表
ALTER TABLE tenants 
ADD COLUMN IF NOT EXISTS subscription_id VARCHAR(255),
ADD COLUMN IF NOT EXISTS stripe_customer_id VARCHAR(255),
ADD COLUMN IF NOT EXISTS industry_template_id UUID,
ADD COLUMN IF NOT EXISTS trial_expires_at TIMESTAMP WITH TIME ZONE;

-- 添加外鍵約束（如果 industry_templates 表存在）
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'industry_templates') THEN
        IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_tenants_industry_template') THEN
            ALTER TABLE tenants
            ADD CONSTRAINT fk_tenants_industry_template
            FOREIGN KEY (industry_template_id) REFERENCES industry_templates(id);
        END IF;
    END IF;
END $$;

-- 添加註釋
COMMENT ON COLUMN tenants.subscription_id IS 'Stripe subscription ID';
COMMENT ON COLUMN tenants.stripe_customer_id IS 'Stripe customer ID';
COMMENT ON COLUMN tenants.industry_template_id IS '行業模板 ID';
COMMENT ON COLUMN tenants.trial_expires_at IS '試用期到期時間';




