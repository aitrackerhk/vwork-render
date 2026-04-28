-- project_resource_reservations：新增使用者（user_id）
ALTER TABLE project_resource_reservations
    ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_prr_user_id ON project_resource_reservations(user_id);


