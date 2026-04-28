-- 為 Admin User 添加員工編號 NUM-000001
UPDATE users
SET employee_number = 'NUM-000001'
WHERE email = 'admin@test.com' AND (employee_number IS NULL OR employee_number = '');

