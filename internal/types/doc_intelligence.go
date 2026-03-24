package types

import (
	"time"

	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────────────────────
// Module 1 – Natural Language Document Operation
// ─────────────────────────────────────────────────────────────────────────────

// DocInstructionRequest is the input for natural language document operations.
type DocInstructionRequest struct {
	// Instruction is the natural-language description of the desired operation
	// (e.g. "translate to English", "extract all dates", "reformat as bullet list").
	Instruction string `json:"instruction" binding:"required"`
	// DocContent is the raw text content of the document to operate on.
	// Either DocContent or a multipart file upload must be provided.
	DocContent string `json:"doc_content"`
	// ModelID is the chat model to use for instruction parsing.
	// When empty the tenant's first available KnowledgeQA model is used.
	ModelID string `json:"model_id"`
}

// DocInstructionResult is the output of a natural language document operation.
type DocInstructionResult struct {
	// LogID is the persisted audit-log record ID.
	LogID string `json:"log_id"`
	// OriginalInstruction is the instruction as received from the user.
	OriginalInstruction string `json:"original_instruction"`
	// ParsedOperation is a plain-English summary of how the LLM interpreted the instruction.
	ParsedOperation string `json:"parsed_operation"`
	// OriginalContent is the document text before the operation.
	OriginalContent string `json:"original_content"`
	// ResultContent is the document text after the operation.
	ResultContent string `json:"result_content"`
}

// DocInstructionLog is the GORM model that persists instruction execution history.
type DocInstructionLog struct {
	ID               string         `gorm:"type:varchar(36);primaryKey"        json:"id"`
	TenantID         uint64         `gorm:"not null;index"                     json:"tenant_id"`
	Instruction      string         `gorm:"type:text;not null;default:''"      json:"instruction"`
	ParsedOperation  string         `gorm:"type:text;not null;default:''"      json:"parsed_operation"`
	OriginalContent  string         `gorm:"type:text;not null;default:''"      json:"original_content"`
	ResultContent    string         `gorm:"type:text;not null;default:''"      json:"result_content"`
	ModelID          string         `gorm:"type:varchar(36);not null;default:''" json:"model_id"`
	Status           string         `gorm:"type:varchar(32);not null;default:'completed'" json:"status"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"index"                              json:"-"`
}

// TableName overrides the GORM default.
func (DocInstructionLog) TableName() string { return "doc_instruction_logs" }

// ─────────────────────────────────────────────────────────────────────────────
// Module 2 – Unstructured Document Information Extraction
// ─────────────────────────────────────────────────────────────────────────────

// DocExtractOptions controls which fields are extracted from a document.
type DocExtractOptions struct {
	// ModelID selects the chat model to use.  Defaults to the tenant default.
	ModelID string `form:"model_id" json:"model_id"`
	// Fields is an optional list of field names to extract.
	// When empty the LLM uses its judgment to extract the most relevant fields.
	Fields []string `form:"fields" json:"fields"`
}

// ExtractedField is a single key→value pair produced by the extraction model.
type ExtractedField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ExtractedData is the GORM model that persists a document extraction result.
type ExtractedData struct {
	ID             string         `gorm:"type:varchar(36);primaryKey"            json:"id"`
	TenantID       uint64         `gorm:"not null;index"                         json:"tenant_id"`
	SourceDocName  string         `gorm:"type:varchar(512);not null;default:''"  json:"source_doc_name"`
	SourceFileType string         `gorm:"type:varchar(32);not null;default:''"   json:"source_file_type"`
	Content        string         `gorm:"type:text;not null;default:''"          json:"content"`
	StructuredData JSON           `gorm:"type:jsonb;not null;default:'{}'"       json:"structured_data"`
	ModelID        string         `gorm:"type:varchar(36);not null;default:''"   json:"model_id"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index"                                  json:"-"`
}

// TableName overrides the GORM default.
func (ExtractedData) TableName() string { return "extracted_data" }

// ExtractedDataResponse is the HTTP response returned to callers.
type ExtractedDataResponse struct {
	ID             string           `json:"id"`
	SourceDocName  string           `json:"source_doc_name"`
	SourceFileType string           `json:"source_file_type"`
	Fields         []ExtractedField `json:"fields"`
	CreatedAt      time.Time        `json:"created_at"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Module 3 – Table Custom Data Filling
// ─────────────────────────────────────────────────────────────────────────────

// TableFillOptions controls the table filling operation.
type TableFillOptions struct {
	// ExtractedDataID references the previously extracted structured data to use
	// as the information source.  Either this field or DocContent must be set.
	ExtractedDataID string `form:"extracted_data_id" json:"extracted_data_id"`
	// DocContent provides inline document text when no extracted_data_id is available.
	DocContent string `form:"doc_content" json:"doc_content"`
	// ModelID selects the chat model.  Defaults to the tenant default.
	ModelID string `form:"model_id" json:"model_id"`
}

// TableFillResult is returned after filling a table template.
type TableFillResult struct {
	// TemplateName is the original template filename.
	TemplateName string `json:"template_name"`
	// FilledFields contains each column/field name and the value written into it.
	FilledFields []ExtractedField `json:"filled_fields"`
	// CSVContent is a CSV representation of the filled table (UTF-8).
	CSVContent string `json:"csv_content"`
	// AccuracyHint is the model's self-reported confidence (0–1).
	AccuracyHint float64 `json:"accuracy_hint"`
}
