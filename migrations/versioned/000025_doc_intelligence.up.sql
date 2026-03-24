-- Migration: 000025_doc_intelligence
-- Description: Add tables for document intelligence modules:
--   1. extracted_data - stores structured data extracted from unstructured documents
--   2. doc_instruction_logs - audit log for natural language instruction executions

DO $$ BEGIN RAISE NOTICE '[Migration 000025] Creating doc intelligence tables'; END $$;

-- Table: extracted_data
-- Stores key information extracted from unstructured documents (DOCX, MD, XLSX, TXT)
-- by the document information extraction module.
CREATE TABLE IF NOT EXISTS extracted_data (
    id             VARCHAR(36)   NOT NULL PRIMARY KEY,
    tenant_id      BIGINT        NOT NULL,
    source_doc_name VARCHAR(512) NOT NULL DEFAULT '',
    source_file_type VARCHAR(32) NOT NULL DEFAULT '',
    content        TEXT          NOT NULL DEFAULT '',
    structured_data JSONB        NOT NULL DEFAULT '{}',
    model_id       VARCHAR(36)   NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ   NOT NULL DEFAULT now(),
    deleted_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_extracted_data_tenant_id ON extracted_data (tenant_id);
CREATE INDEX IF NOT EXISTS idx_extracted_data_deleted_at ON extracted_data (deleted_at);

COMMENT ON TABLE  extracted_data IS 'Structured data extracted from unstructured documents by the document intelligence module';
COMMENT ON COLUMN extracted_data.source_doc_name  IS 'Original filename of the source document';
COMMENT ON COLUMN extracted_data.source_file_type IS 'File type: docx, md, xlsx, txt, etc.';
COMMENT ON COLUMN extracted_data.content          IS 'Full parsed text content of the source document';
COMMENT ON COLUMN extracted_data.structured_data  IS 'LLM-extracted key fields stored as JSON';
COMMENT ON COLUMN extracted_data.model_id         IS 'ID of the chat model used for extraction';

-- Table: doc_instruction_logs
-- Audit log for natural language document operation instructions.
CREATE TABLE IF NOT EXISTS doc_instruction_logs (
    id                  VARCHAR(36)  NOT NULL PRIMARY KEY,
    tenant_id           BIGINT       NOT NULL,
    instruction         TEXT         NOT NULL DEFAULT '',
    parsed_operation    TEXT         NOT NULL DEFAULT '',
    original_content    TEXT         NOT NULL DEFAULT '',
    result_content      TEXT         NOT NULL DEFAULT '',
    model_id            VARCHAR(36)  NOT NULL DEFAULT '',
    status              VARCHAR(32)  NOT NULL DEFAULT 'completed',
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at          TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_doc_instruction_logs_tenant_id ON doc_instruction_logs (tenant_id);
CREATE INDEX IF NOT EXISTS idx_doc_instruction_logs_deleted_at ON doc_instruction_logs (deleted_at);

COMMENT ON TABLE  doc_instruction_logs IS 'Audit log for natural language document operation instructions';
COMMENT ON COLUMN doc_instruction_logs.instruction      IS 'The original natural language instruction from the user';
COMMENT ON COLUMN doc_instruction_logs.parsed_operation IS 'LLM-interpreted operation description';
COMMENT ON COLUMN doc_instruction_logs.original_content IS 'Document content before the operation';
COMMENT ON COLUMN doc_instruction_logs.result_content   IS 'Document content after the operation';
COMMENT ON COLUMN doc_instruction_logs.status           IS 'Operation status: completed or failed';

DO $$ BEGIN RAISE NOTICE '[Migration 000025] Doc intelligence tables created successfully'; END $$;
