ALTER TABLE payrolls
    ADD COLUMN IF NOT EXISTS employee_mandatory_rate DECIMAL(6,4) DEFAULT 0.05,
    ADD COLUMN IF NOT EXISTS employer_mandatory_rate DECIMAL(6,4) DEFAULT 0.05;

