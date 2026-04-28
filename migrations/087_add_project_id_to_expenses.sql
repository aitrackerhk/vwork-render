-- expenses：關聯項目（project）
ALTER TABLE expenses
    ADD COLUMN IF NOT EXISTS project_id UUID REFERENCES projects(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_expenses_project_id ON expenses(project_id);


