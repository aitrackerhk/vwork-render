-- Add posting rule table and source linkage columns

ALTER TABLE incomes
    ADD COLUMN IF NOT EXISTS journal_entry_id UUID NULL;

ALTER TABLE expenses
    ADD COLUMN IF NOT EXISTS journal_entry_id UUID NULL;

CREATE INDEX IF NOT EXISTS idx_incomes_journal_entry_id ON incomes(journal_entry_id);
CREATE INDEX IF NOT EXISTS idx_expenses_journal_entry_id ON expenses(journal_entry_id);

CREATE TABLE IF NOT EXISTS posting_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    source_type VARCHAR(20) NOT NULL,
    category VARCHAR(100) NOT NULL,
    debit_account_id UUID NOT NULL,
    credit_account_id UUID NOT NULL,
    description TEXT NULL,
    is_system BOOLEAN DEFAULT FALSE,
    is_active BOOLEAN DEFAULT TRUE,
    sort_order INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT fk_posting_rules_debit FOREIGN KEY (debit_account_id) REFERENCES accounts(id) ON DELETE RESTRICT,
    CONSTRAINT fk_posting_rules_credit FOREIGN KEY (credit_account_id) REFERENCES accounts(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_posting_rules_tenant_type_category ON posting_rules(tenant_id, source_type, category);
