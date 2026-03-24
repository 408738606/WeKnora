package handler

import (
	"net/http"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	secutils "github.com/Tencent/WeKnora/internal/utils"
	"github.com/gin-gonic/gin"
)

// DocIntelligenceHandler handles the three document intelligence modules:
//  1. Natural language document operation
//  2. Unstructured document information extraction
//  3. Table custom data filling
type DocIntelligenceHandler struct {
	svc interfaces.DocIntelligenceService
}

// NewDocIntelligenceHandler creates a new DocIntelligenceHandler.
func NewDocIntelligenceHandler(svc interfaces.DocIntelligenceService) *DocIntelligenceHandler {
	return &DocIntelligenceHandler{svc: svc}
}

// ─────────────────────────────────────────────────────────────────────────────
// Module 1 – Natural Language Document Operation
// ─────────────────────────────────────────────────────────────────────────────

// ExecuteInstruction godoc
// @Summary      Execute a natural-language document operation
// @Description  Parses a natural-language instruction and applies it to the
//
//	provided document content (or to an uploaded file).
//	Supported operations: edit, reformat, translate, summarise, extract, etc.
//
// @Tags         doc-intelligence
// @Accept       multipart/form-data
// @Produce      json
// @Param        instruction  formData  string  true   "Natural-language instruction"
// @Param        doc_content  formData  string  false  "Plain-text document content (alternative to file)"
// @Param        model_id     formData  string  false  "Chat model ID (uses tenant default when omitted)"
// @Param        file         formData  file    false  "Document file (.docx, .md, .xlsx, .txt)"
// @Success      200  {object}  types.DocInstructionResult
// @Failure      400  {object}  errors.AppError
// @Failure      500  {object}  errors.AppError
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /doc-intelligence/instructions [post]
func (h *DocIntelligenceHandler) ExecuteInstruction(c *gin.Context) {
	ctx := c.Request.Context()

	req := &types.DocInstructionRequest{
		Instruction: c.PostForm("instruction"),
		DocContent:  c.PostForm("doc_content"),
		ModelID:     c.PostForm("model_id"),
	}
	req.Instruction = secutils.SanitizeForLog(req.Instruction)
	if req.Instruction == "" {
		c.Error(errors.NewBadRequestError("instruction is required"))
		return
	}

	// Optional file upload.
	fileHeader, _ := c.FormFile("file")
	if fileHeader != nil {
		maxSize := secutils.GetMaxFileSize()
		if fileHeader.Size > maxSize {
			c.Error(errors.NewBadRequestError("file too large"))
			return
		}
	}

	logger.Infof(ctx, "[DocIntelligence] ExecuteInstruction: %q", req.Instruction)

	result, err := h.svc.ExecuteInstruction(ctx, req, fileHeader)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// Module 2 – Document Information Extraction
// ─────────────────────────────────────────────────────────────────────────────

// ExtractFromFile godoc
// @Summary      Extract key information from an unstructured document
// @Description  Uploads and parses a document (DOCX, MD, XLSX, TXT), then uses
//
//	an LLM to extract key fields and persists the result.
//
// @Tags         doc-intelligence
// @Accept       multipart/form-data
// @Produce      json
// @Param        file      formData  file    true   "Document file (.docx, .md, .xlsx, .txt)"
// @Param        model_id  formData  string  false  "Chat model ID"
// @Param        fields    formData  string  false  "Comma-separated list of field names to extract"
// @Success      200  {object}  types.ExtractedDataResponse
// @Failure      400  {object}  errors.AppError
// @Failure      500  {object}  errors.AppError
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /doc-intelligence/extract [post]
func (h *DocIntelligenceHandler) ExtractFromFile(c *gin.Context) {
	ctx := c.Request.Context()

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.Error(errors.NewBadRequestError("file is required"))
		return
	}
	maxSize := secutils.GetMaxFileSize()
	if fileHeader.Size > maxSize {
		c.Error(errors.NewBadRequestError("file too large"))
		return
	}

	opts := &types.DocExtractOptions{
		ModelID: c.PostForm("model_id"),
	}
	if fieldsStr := c.PostForm("fields"); fieldsStr != "" {
		for _, f := range splitCSV(fieldsStr) {
			f = secutils.SanitizeForLog(f)
			if f != "" {
				opts.Fields = append(opts.Fields, f)
			}
		}
	}

	logger.Infof(ctx, "[DocIntelligence] ExtractFromFile: %q fields=%v", fileHeader.Filename, opts.Fields)

	resp, err := h.svc.ExtractFromFile(ctx, fileHeader, opts)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    resp,
	})
}

// ListExtractedData godoc
// @Summary      List all extracted data records for the current tenant
// @Tags         doc-intelligence
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Failure      500  {object}  errors.AppError
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /doc-intelligence/extracted-data [get]
func (h *DocIntelligenceHandler) ListExtractedData(c *gin.Context) {
	ctx := c.Request.Context()

	records, err := h.svc.ListExtractedData(ctx)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    records,
		"total":   len(records),
	})
}

// GetExtractedData godoc
// @Summary      Get a single extracted data record
// @Tags         doc-intelligence
// @Produce      json
// @Param        id  path  string  true  "Extracted data ID"
// @Success      200  {object}  types.ExtractedData
// @Failure      404  {object}  errors.AppError
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /doc-intelligence/extracted-data/{id} [get]
func (h *DocIntelligenceHandler) GetExtractedData(c *gin.Context) {
	ctx := c.Request.Context()
	id := secutils.SanitizeForLog(c.Param("id"))
	if id == "" {
		c.Error(errors.NewBadRequestError("id is required"))
		return
	}

	record, err := h.svc.GetExtractedData(ctx, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewNotFoundError("extracted data not found"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    record,
	})
}

// DeleteExtractedData godoc
// @Summary      Delete an extracted data record
// @Tags         doc-intelligence
// @Produce      json
// @Param        id  path  string  true  "Extracted data ID"
// @Success      200  {object}  map[string]interface{}
// @Failure      500  {object}  errors.AppError
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /doc-intelligence/extracted-data/{id} [delete]
func (h *DocIntelligenceHandler) DeleteExtractedData(c *gin.Context) {
	ctx := c.Request.Context()
	id := secutils.SanitizeForLog(c.Param("id"))
	if id == "" {
		c.Error(errors.NewBadRequestError("id is required"))
		return
	}

	if err := h.svc.DeleteExtractedData(ctx, id); err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ─────────────────────────────────────────────────────────────────────────────
// Module 3 – Table Custom Data Filling
// ─────────────────────────────────────────────────────────────────────────────

// FillTable godoc
// @Summary      Fill a table template with extracted document data
// @Description  Uploads a table template (Excel/Word/CSV), sources field values
//
//	from a prior extraction record or inline doc_content, and returns
//	the filled table as a JSON mapping plus a CSV download.
//	Response time target: ≤ 90 seconds.  Fill accuracy target: ≥ 80%.
//
// @Tags         doc-intelligence
// @Accept       multipart/form-data
// @Produce      json
// @Param        file              formData  file    true   "Table template file (.xlsx, .docx, .csv)"
// @Param        extracted_data_id formData  string  false  "ID of a prior extraction record to use as source"
// @Param        doc_content       formData  string  false  "Inline document text (alternative to extracted_data_id)"
// @Param        model_id          formData  string  false  "Chat model ID"
// @Success      200  {object}  types.TableFillResult
// @Failure      400  {object}  errors.AppError
// @Failure      500  {object}  errors.AppError
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /doc-intelligence/fill-table [post]
func (h *DocIntelligenceHandler) FillTable(c *gin.Context) {
	ctx := c.Request.Context()

	templateHeader, err := c.FormFile("file")
	if err != nil {
		c.Error(errors.NewBadRequestError("file (table template) is required"))
		return
	}
	maxSize := secutils.GetMaxFileSize()
	if templateHeader.Size > maxSize {
		c.Error(errors.NewBadRequestError("file too large"))
		return
	}

	opts := &types.TableFillOptions{
		ExtractedDataID: secutils.SanitizeForLog(c.PostForm("extracted_data_id")),
		DocContent:      c.PostForm("doc_content"),
		ModelID:         c.PostForm("model_id"),
	}

	if opts.ExtractedDataID == "" && opts.DocContent == "" {
		c.Error(errors.NewBadRequestError("either extracted_data_id or doc_content is required"))
		return
	}

	logger.Infof(ctx, "[DocIntelligence] FillTable template=%q extractedDataID=%q",
		templateHeader.Filename, opts.ExtractedDataID)

	result, err := h.svc.FillTable(ctx, templateHeader, opts)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// Utility
// ─────────────────────────────────────────────────────────────────────────────

// splitCSV splits a comma-separated string and trims whitespace from each element.
func splitCSV(s string) []string {
	parts := make([]string, 0)
	for _, p := range splitOn(s, ',') {
		p = trimSpace(p)
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

func splitOn(s string, sep rune) []string {
	var parts []string
	start := 0
	for i, r := range s {
		if r == sep {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func trimSpace(s string) string {
	return trimChars(s, " \t\n\r")
}

func trimChars(s, chars string) string {
	for len(s) > 0 {
		found := false
		for _, c := range chars {
			if rune(s[0]) == c {
				s = s[1:]
				found = true
				break
			}
		}
		if !found {
			break
		}
	}
	for len(s) > 0 {
		found := false
		for _, c := range chars {
			if rune(s[len(s)-1]) == c {
				s = s[:len(s)-1]
				found = true
				break
			}
		}
		if !found {
			break
		}
	}
	return s
}
