-- 修改優惠券有效期字段：從 date 改為 timestamp with time zone，並允許 valid_to 為空（一直有效）
ALTER TABLE coupons
    ALTER COLUMN valid_from TYPE TIMESTAMP WITH TIME ZONE USING valid_from::TIMESTAMP WITH TIME ZONE,
    ALTER COLUMN valid_to TYPE TIMESTAMP WITH TIME ZONE USING 
        CASE 
            WHEN valid_to IS NULL THEN NULL
            ELSE valid_to::TIMESTAMP WITH TIME ZONE
        END;

-- 移除 valid_to 的 NOT NULL 約束（允許為空表示一直有效）
ALTER TABLE coupons
    ALTER COLUMN valid_to DROP NOT NULL;

