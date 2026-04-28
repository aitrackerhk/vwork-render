-- projects：新增封面與類型
ALTER TABLE projects
    ADD COLUMN IF NOT EXISTS cover_url VARCHAR(500);

ALTER TABLE projects
    ADD COLUMN IF NOT EXISTS project_type_id UUID REFERENCES project_types(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_projects_project_type ON projects(project_type_id);


