-- POS: default status should be 'confirmed' (已確認), not 'completed' (已完成)

ALTER TABLE pos_sales
ALTER COLUMN status SET DEFAULT 'confirmed';


