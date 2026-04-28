-- Fix module_code inconsistency: project vs projects
-- 項目管理模塊代碼統一為 "projects"

-- 若同一 tenant 同時存在 project 與 projects，先刪除舊的 project，避免 UNIQUE(tenant_id, module_code) 衝突
DELETE FROM tenant_modules tm
USING tenant_modules tm2
WHERE tm.tenant_id = tm2.tenant_id
  AND tm.module_code = 'project'
  AND tm2.module_code = 'projects';

-- 將 project 統一更新為 projects
UPDATE tenant_modules
SET module_code = 'projects',
    updated_at = NOW()
WHERE module_code = 'project';
















