-- Add trial months field to sales partner applications
-- This allows sales partners to set custom trial period for tenants using their code

ALTER TABLE sales_partner_applications
    ADD COLUMN IF NOT EXISTS trial_months INTEGER NULL;

-- Add comment to explain the field
COMMENT ON COLUMN sales_partner_applications.trial_months IS 'Custom trial period in months for tenants using this sales partner code. Max 2 months. NULL means using system default trial.';

