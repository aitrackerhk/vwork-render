-- 添加 income_id 字段到 orders 表的 extra_fields 中的 payment_records
-- 注意：这个迁移主要是为了确保数据结构支持，实际的数据迁移需要在应用层处理

-- 如果需要，可以添加一个触发器或函数来自动创建 income 记录
-- 但为了灵活性，我们将在应用层处理这个逻辑

