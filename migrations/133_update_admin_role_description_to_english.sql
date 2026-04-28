-- Update legacy Admin role description written in Chinese to English default
-- This keeps existing tenants consistent after the code default was switched to English.

UPDATE roles
SET
    description = 'System administrator with full permissions',
    updated_at = CURRENT_TIMESTAMP
WHERE
    description = '系统管理员，拥有所有权限'
    AND (
        LOWER(name) = 'admin'
        OR name IN ('Admin', '管理员', '管理員')
    );


