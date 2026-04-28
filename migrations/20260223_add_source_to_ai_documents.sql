-- Add source column to ai_documents to distinguish docs-page vs chat-generated documents
ALTER TABLE ai_documents ADD COLUMN IF NOT EXISTS source VARCHAR(20) DEFAULT 'docs';
