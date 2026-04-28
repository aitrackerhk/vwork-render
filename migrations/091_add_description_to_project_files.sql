-- project_files：新增描述欄位
ALTER TABLE project_files
    ADD COLUMN IF NOT EXISTS description TEXT;


