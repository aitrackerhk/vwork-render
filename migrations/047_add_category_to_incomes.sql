-- 添加类别字段到 incomes 表（如果还没有）
-- 注意：category 字段已经存在，这里只是确保它存在
-- 添加 income_id 到 orders 表的 extra_fields 中的 payment_records

-- 如果需要，可以添加索引
CREATE INDEX IF NOT EXISTS idx_incomes_category ON incomes(category) WHERE category IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_incomes_reference_id ON incomes(reference_id) WHERE reference_id IS NOT NULL;

