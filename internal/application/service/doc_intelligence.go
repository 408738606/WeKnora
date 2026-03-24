package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"path/filepath"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────────────────────
// Prompt templates
// ─────────────────────────────────────────────────────────────────────────────

const docInstructionPrompt = `You are an intelligent document editing assistant.
The user has provided a document and a natural-language instruction describing an operation to perform.

Instruction: %s

Document content:
"""
%s
"""

Tasks:
1. In ONE sentence, describe exactly what operation you will perform (field: "parsed_operation").
2. Apply the operation to the document content and return the full result (field: "result_content").

Respond ONLY with valid JSON in this exact schema:
{
  "parsed_operation": "<one-sentence description>",
  "result_content": "<full modified document text>"
}`

const docExtractPrompt = `You are a document information extraction specialist.
Extract key information from the following document content.

%s

Document content:
"""
%s
"""

Return ONLY valid JSON in this exact schema – no markdown, no extra text:
{
  "fields": [
    {"name": "<field name>", "value": "<extracted value>"},
    ...
  ]
}`

const tableFillPrompt = `You are a data entry assistant.
Given the following SOURCE INFORMATION and a TABLE TEMPLATE, fill in every column of the table.

SOURCE INFORMATION:
"""
%s
"""

TABLE TEMPLATE (column names, one per line):
%s

Rules:
- Match each column name to the most relevant piece of information in the source.
- If no matching information exists, use an empty string "".
- Self-assess your overall fill confidence on a 0-to-1 scale.

Return ONLY valid JSON – no markdown, no extra text:
{
  "filled_fields": [
    {"name": "<column name>", "value": "<filled value>"},
    ...
  ],
  "accuracy_hint": <float 0-1>
}`

// ─────────────────────────────────────────────────────────────────────────────
// docIntelligenceService
// ─────────────────────────────────────────────────────────────────────────────

type docIntelligenceService struct {
	db             *gorm.DB
	documentReader interfaces.DocumentReader
	modelService   interfaces.ModelService
}

// NewDocIntelligenceService wires up the DocIntelligenceService.
func NewDocIntelligenceService(
	db *gorm.DB,
	documentReader interfaces.DocumentReader,
	modelService interfaces.ModelService,
) interfaces.DocIntelligenceService {
	return &docIntelligenceService{
		db:             db,
		documentReader: documentReader,
		modelService:   modelService,
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

// readFileHeader reads and parses a multipart.FileHeader into plain text using
// the configured document reader (gRPC docreader service).
func (s *docIntelligenceService) readFileHeader(ctx context.Context, fh *multipart.FileHeader) (string, error) {
	f, err := fh.Open()
	if err != nil {
		return "", fmt.Errorf("open uploaded file: %w", err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return "", fmt.Errorf("read uploaded file: %w", err)
	}

	req := &types.ReadRequest{
		FileContent: data,
		FileName:    fh.Filename,
		FileType:    strings.TrimPrefix(filepath.Ext(fh.Filename), "."),
		RequestID:   uuid.New().String(),
	}
	result, err := s.documentReader.Read(ctx, req)
	if err != nil {
		return "", fmt.Errorf("parse document: %w", err)
	}
	if result.Error != "" {
		return "", fmt.Errorf("document reader error: %s", result.Error)
	}
	return result.MarkdownContent, nil
}

// getChatModel resolves the model to use.  Falls back to the first available
// KnowledgeQA model when modelID is empty.
func (s *docIntelligenceService) getChatModel(ctx context.Context, modelID string) (chat.Chat, error) {
	if modelID == "" {
		// Pick the first available KnowledgeQA model for this tenant.
		models, err := s.modelService.ListModels(ctx)
		if err != nil {
			return nil, fmt.Errorf("list models: %w", err)
		}
		for _, m := range models {
			if m.Type == types.ModelTypeKnowledgeQA {
				modelID = m.ID
				break
			}
		}
	}
	if modelID == "" {
		return nil, fmt.Errorf("no KnowledgeQA model available for this tenant; please configure one first")
	}
	return s.modelService.GetChatModel(ctx, modelID)
}

// callLLM sends a single user message to the chat model and returns the reply.
func callLLM(ctx context.Context, chatModel chat.Chat, prompt string) (string, error) {
	thinking := false
	resp, err := chatModel.Chat(ctx, []chat.Message{
		{Role: "user", Content: prompt},
	}, &chat.ChatOptions{
		Temperature: 0.2,
		MaxTokens:   4096,
		Thinking:    &thinking,
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// stripMarkdownFence removes the ```json ... ``` wrapper that some models add.
func stripMarkdownFence(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		// remove first line (``` or ```json) and last line (```)
		lines := strings.SplitN(s, "\n", 2)
		if len(lines) == 2 {
			s = lines[1]
		}
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	}
	return strings.TrimSpace(s)
}

// ─────────────────────────────────────────────────────────────────────────────
// Module 1 – Natural Language Document Operation
// ─────────────────────────────────────────────────────────────────────────────

func (s *docIntelligenceService) ExecuteInstruction(
	ctx context.Context,
	req *types.DocInstructionRequest,
	fileHeader *multipart.FileHeader,
) (*types.DocInstructionResult, error) {
	tenantID := types.MustTenantIDFromContext(ctx)
	logger.Infof(ctx, "[DocIntelligence] ExecuteInstruction tenant=%d instruction=%q", tenantID, req.Instruction)

	// 1. Obtain document text.
	docContent := req.DocContent
	if docContent == "" && fileHeader != nil {
		var err error
		docContent, err = s.readFileHeader(ctx, fileHeader)
		if err != nil {
			return nil, fmt.Errorf("read document file: %w", err)
		}
	}
	if docContent == "" {
		return nil, fmt.Errorf("either doc_content or a file upload is required")
	}

	// 2. Resolve chat model.
	chatModel, err := s.getChatModel(ctx, req.ModelID)
	if err != nil {
		return nil, err
	}

	// 3. Call LLM.
	prompt := fmt.Sprintf(docInstructionPrompt, req.Instruction, docContent)
	raw, err := callLLM(ctx, chatModel, prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	// 4. Parse JSON response.
	raw = stripMarkdownFence(raw)
	var llmResp struct {
		ParsedOperation string `json:"parsed_operation"`
		ResultContent   string `json:"result_content"`
	}
	if err := json.Unmarshal([]byte(raw), &llmResp); err != nil {
		// Graceful fallback: treat entire response as result_content.
		logger.Warnf(ctx, "[DocIntelligence] LLM returned non-JSON; using raw as result: %v", err)
		llmResp.ParsedOperation = req.Instruction
		llmResp.ResultContent = raw
	}

	// 5. Persist audit log.
	logID := uuid.New().String()
	logEntry := &types.DocInstructionLog{
		ID:              logID,
		TenantID:        tenantID,
		Instruction:     req.Instruction,
		ParsedOperation: llmResp.ParsedOperation,
		OriginalContent: docContent,
		ResultContent:   llmResp.ResultContent,
		ModelID:         chatModel.GetModelID(),
		Status:          "completed",
	}
	if dbErr := s.db.WithContext(ctx).Create(logEntry).Error; dbErr != nil {
		logger.Warnf(ctx, "[DocIntelligence] Failed to persist instruction log: %v", dbErr)
	}

	return &types.DocInstructionResult{
		LogID:               logID,
		OriginalInstruction: req.Instruction,
		ParsedOperation:     llmResp.ParsedOperation,
		OriginalContent:     docContent,
		ResultContent:       llmResp.ResultContent,
	}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Module 2 – Document Information Extraction
// ─────────────────────────────────────────────────────────────────────────────

func (s *docIntelligenceService) ExtractFromFile(
	ctx context.Context,
	fileHeader *multipart.FileHeader,
	opts *types.DocExtractOptions,
) (*types.ExtractedDataResponse, error) {
	tenantID := types.MustTenantIDFromContext(ctx)
	logger.Infof(ctx, "[DocIntelligence] ExtractFromFile tenant=%d file=%q", tenantID, fileHeader.Filename)

	// 1. Parse document to text.
	docContent, err := s.readFileHeader(ctx, fileHeader)
	if err != nil {
		return nil, fmt.Errorf("parse document: %w", err)
	}

	// 2. Resolve chat model.
	chatModel, err := s.getChatModel(ctx, opts.ModelID)
	if err != nil {
		return nil, err
	}

	// 3. Build field-hint text.
	fieldHint := ""
	if len(opts.Fields) > 0 {
		fieldHint = "Extract ONLY the following fields: " + strings.Join(opts.Fields, ", ") + "."
	} else {
		fieldHint = "Extract all key information, entities, dates, amounts, names, and important facts."
	}

	// 4. Call LLM.
	prompt := fmt.Sprintf(docExtractPrompt, fieldHint, docContent)
	raw, err := callLLM(ctx, chatModel, prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM extraction failed: %w", err)
	}

	// 5. Parse JSON response.
	raw = stripMarkdownFence(raw)
	var llmResp struct {
		Fields []types.ExtractedField `json:"fields"`
	}
	if err := json.Unmarshal([]byte(raw), &llmResp); err != nil {
		logger.Warnf(ctx, "[DocIntelligence] LLM returned non-JSON for extraction: %v; raw=%s", err, raw)
		// Build a single fallback field.
		llmResp.Fields = []types.ExtractedField{{Name: "content", Value: raw}}
	}

	// 6. Serialize structured data for storage.
	structuredBytes, _ := json.Marshal(llmResp.Fields)
	var structuredJSON types.JSON
	_ = json.Unmarshal(structuredBytes, &structuredJSON)

	// 7. Persist.
	fileType := strings.ToLower(strings.TrimPrefix(filepath.Ext(fileHeader.Filename), "."))
	record := &types.ExtractedData{
		ID:             uuid.New().String(),
		TenantID:       tenantID,
		SourceDocName:  fileHeader.Filename,
		SourceFileType: fileType,
		Content:        docContent,
		StructuredData: structuredJSON,
		ModelID:        chatModel.GetModelID(),
	}
	if dbErr := s.db.WithContext(ctx).Create(record).Error; dbErr != nil {
		return nil, fmt.Errorf("persist extracted data: %w", dbErr)
	}

	return &types.ExtractedDataResponse{
		ID:             record.ID,
		SourceDocName:  record.SourceDocName,
		SourceFileType: record.SourceFileType,
		Fields:         llmResp.Fields,
		CreatedAt:      record.CreatedAt,
	}, nil
}

func (s *docIntelligenceService) ListExtractedData(ctx context.Context) ([]*types.ExtractedData, error) {
	tenantID := types.MustTenantIDFromContext(ctx)
	var records []*types.ExtractedData
	if err := s.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("created_at DESC").
		Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

func (s *docIntelligenceService) GetExtractedData(ctx context.Context, id string) (*types.ExtractedData, error) {
	tenantID := types.MustTenantIDFromContext(ctx)
	var record types.ExtractedData
	if err := s.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		First(&record).Error; err != nil {
		return nil, err
	}
	return &record, nil
}

func (s *docIntelligenceService) DeleteExtractedData(ctx context.Context, id string) error {
	tenantID := types.MustTenantIDFromContext(ctx)
	return s.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Delete(&types.ExtractedData{}).Error
}

// ─────────────────────────────────────────────────────────────────────────────
// Module 3 – Table Custom Data Filling
// ─────────────────────────────────────────────────────────────────────────────

func (s *docIntelligenceService) FillTable(
	ctx context.Context,
	templateHeader *multipart.FileHeader,
	opts *types.TableFillOptions,
) (*types.TableFillResult, error) {
	tenantID := types.MustTenantIDFromContext(ctx)
	logger.Infof(ctx, "[DocIntelligence] FillTable tenant=%d template=%q", tenantID, templateHeader.Filename)

	// 1. Parse template to extract column headers.
	templateText, err := s.readFileHeader(ctx, templateHeader)
	if err != nil {
		return nil, fmt.Errorf("parse table template: %w", err)
	}
	columns := extractColumns(templateText)
	if len(columns) == 0 {
		return nil, fmt.Errorf("no column headers found in template; ensure the first row contains field names")
	}

	// 2. Obtain source information.
	var sourceInfo string
	if opts.ExtractedDataID != "" {
		record, err := s.GetExtractedData(ctx, opts.ExtractedDataID)
		if err != nil {
			return nil, fmt.Errorf("load extracted data %s: %w", opts.ExtractedDataID, err)
		}
		// Marshal structured_data back to human-readable form.
		var fields []types.ExtractedField
		rawJSON, _ := json.Marshal(record.StructuredData)
		if err := json.Unmarshal(rawJSON, &fields); err == nil && len(fields) > 0 {
			var sb strings.Builder
			for _, f := range fields {
				sb.WriteString(f.Name)
				sb.WriteString(": ")
				sb.WriteString(f.Value)
				sb.WriteString("\n")
			}
			sourceInfo = sb.String()
		} else {
			sourceInfo = record.Content
		}
	} else if opts.DocContent != "" {
		sourceInfo = opts.DocContent
	}
	if sourceInfo == "" {
		return nil, fmt.Errorf("either extracted_data_id or doc_content is required as information source")
	}

	// 3. Resolve chat model.
	chatModel, err := s.getChatModel(ctx, opts.ModelID)
	if err != nil {
		return nil, err
	}

	// 4. Call LLM to fill the table.
	columnList := strings.Join(columns, "\n")
	prompt := fmt.Sprintf(tableFillPrompt, sourceInfo, columnList)
	raw, err := callLLM(ctx, chatModel, prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM table fill failed: %w", err)
	}

	// 5. Parse LLM response.
	raw = stripMarkdownFence(raw)
	var llmResp struct {
		FilledFields []types.ExtractedField `json:"filled_fields"`
		AccuracyHint float64                `json:"accuracy_hint"`
	}
	if err := json.Unmarshal([]byte(raw), &llmResp); err != nil {
		logger.Warnf(ctx, "[DocIntelligence] LLM returned non-JSON for table fill: %v", err)
		// Fallback: one field per column with empty value.
		for _, col := range columns {
			llmResp.FilledFields = append(llmResp.FilledFields, types.ExtractedField{Name: col, Value: ""})
		}
		llmResp.AccuracyHint = 0
	}

	// 6. Build CSV output.
	csvContent, err := buildCSV(llmResp.FilledFields)
	if err != nil {
		logger.Warnf(ctx, "[DocIntelligence] Failed to build CSV: %v", err)
		csvContent = ""
	}

	// 7. Clamp accuracy hint.
	if llmResp.AccuracyHint < 0 {
		llmResp.AccuracyHint = 0
	}
	if llmResp.AccuracyHint > 1 {
		llmResp.AccuracyHint = 1
	}

	return &types.TableFillResult{
		TemplateName: templateHeader.Filename,
		FilledFields: llmResp.FilledFields,
		CSVContent:   csvContent,
		AccuracyHint: llmResp.AccuracyHint,
	}, nil
}

// extractColumns parses plain text (as produced by the docreader from Excel/Word/CSV)
// and returns a de-duplicated list of column-header candidates.
//
// Strategy:
//  1. Look for a Markdown table header row: | col1 | col2 | ...
//  2. Fall back to the first non-empty line, split by common delimiters.
func extractColumns(text string) []string {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "|") && strings.HasSuffix(line, "|") {
			parts := strings.Split(line, "|")
			var cols []string
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" && p != "---" && !strings.Contains(p, "---") {
					cols = append(cols, p)
				}
			}
			if len(cols) > 0 {
				return cols
			}
		}
	}
	// Fallback: first non-empty line, tab/comma separated.
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var parts []string
		if strings.Contains(line, "\t") {
			parts = strings.Split(line, "\t")
		} else if strings.Contains(line, ",") {
			r := csv.NewReader(strings.NewReader(line))
			record, err := r.Read()
			if err == nil {
				parts = record
			}
		} else {
			parts = []string{line}
		}
		var cols []string
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				cols = append(cols, p)
			}
		}
		if len(cols) > 0 {
			return cols
		}
	}
	return nil
}

// buildCSV generates a two-row CSV (header + values) from a slice of ExtractedField.
func buildCSV(fields []types.ExtractedField) (string, error) {
	if len(fields) == 0 {
		return "", nil
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	headers := make([]string, len(fields))
	values := make([]string, len(fields))
	for i, f := range fields {
		headers[i] = f.Name
		values[i] = f.Value
	}
	if err := w.Write(headers); err != nil {
		return "", err
	}
	if err := w.Write(values); err != nil {
		return "", err
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return "", err
	}
	return buf.String(), nil
}
