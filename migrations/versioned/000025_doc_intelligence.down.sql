-- Rollback migration: 000025_doc_intelligence
DO $$ BEGIN RAISE NOTICE '[Migration 000025 rollback] Dropping doc intelligence tables'; END $$;

DROP TABLE IF EXISTS doc_instruction_logs;
DROP TABLE IF EXISTS extracted_data;

DO $$ BEGIN RAISE NOTICE '[Migration 000025 rollback] Doc intelligence tables dropped'; END $$;
