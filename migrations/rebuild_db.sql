-- vWork 完整資料庫重建腳本
-- 使用方法：psql -U postgres -d postgres -f rebuild_db.sql

-- 刪除現有資料庫（如果存在）
DROP DATABASE IF EXISTS "u-nai";

-- 創建新資料庫
CREATE DATABASE "u-nai";

-- 連接到新資料庫
\c "u-nai"

-- 執行初始架構
\i 001_initial_schema.sql

-- 執行 CMS 模組架構
\i 002_cms_modules_schema.sql

-- 完成
\echo '✅ Database rebuild completed successfully!'

