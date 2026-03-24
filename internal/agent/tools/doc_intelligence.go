package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/utils"
)

// ─────────────────────────────────────────────────────────────────────────────
// Tool: doc_instruction
// ─────────────────────────────────────────────────────────────────────────────

var docInstructionToolDef = BaseTool{
	name: ToolDocInstruction,
	description: `Apply a natural-language editing or transformation instruction to a piece of document text.

Use this tool when the user asks you to:
- Edit, rewrite, or rephrase a piece of text
- Translate document content to another language
- Summarise, expand, or reformulate text
- Change the formatting, tone, or structure of a document passage
- Extract specific information from a passage (dates, names, numbers, etc.)

Provide the full document text in **content** and describe the desired transformation in **instruction**.
The tool returns the modified text so you can present it to the user in the chat.`,
	schema: utils.GenerateSchema[DocInstructionInput](),
}

// DocInstructionInput is the structured input for the doc_instruction tool.
type DocInstructionInput struct {
	Content     string `json:"content"     jsonschema:"required,The document text to operate on"`
	Instruction string `json:"instruction" jsonschema:"required,Natural-language description of the operation to perform"`
}

// DocInstructionTool applies a natural-language instruction to document content.
type DocInstructionTool struct {
	BaseTool
	chatModel chat.Chat
}

// NewDocInstructionTool creates a new DocInstructionTool.
func NewDocInstructionTool(chatModel chat.Chat) *DocInstructionTool {
	return &DocInstructionTool{
		BaseTool:  docInstructionToolDef,
		chatModel: chatModel,
	}
}

// Execute runs the instruction against the provided content.
func (t *DocInstructionTool) Execute(ctx context.Context, args json.RawMessage) (*types.ToolResult, error) {
	var input DocInstructionInput
	if err := json.Unmarshal(args, &input); err != nil {
		return &types.ToolResult{Success: false, Error: fmt.Sprintf("invalid args: %v", err)}, err
	}
	if strings.TrimSpace(input.Content) == "" {
		return &types.ToolResult{Success: false, Error: "content is required"}, nil
	}
	if strings.TrimSpace(input.Instruction) == "" {
		return &types.ToolResult{Success: false, Error: "instruction is required"}, nil
	}

	prompt := fmt.Sprintf(`You are a document editing assistant.

Instruction: %s

Document content:
"""
%s
"""

Apply the instruction precisely and return ONLY the resulting document text — no commentary, no JSON, no markdown fences.`, input.Instruction, input.Content)

	thinking := false
	resp, err := t.chatModel.Chat(ctx, []chat.Message{
		{Role: "user", Content: prompt},
	}, &chat.ChatOptions{
		Temperature: 0.2,
		MaxTokens:   4096,
		Thinking:    &thinking,
	})
	if err != nil {
		logger.Errorf(ctx, "[Tool][DocInstruction] LLM call failed: %v", err)
		return &types.ToolResult{Success: false, Error: fmt.Sprintf("LLM error: %v", err)}, err
	}

	result := strings.TrimSpace(resp.Content)
	return &types.ToolResult{
		Success: true,
		Output:  result,
		Data: map[string]interface{}{
			"display_type":        ToolDocInstruction,
			"original_content":    input.Content,
			"instruction":         input.Instruction,
			"result_content":      result,
		},
	}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Tool: doc_extract
// ─────────────────────────────────────────────────────────────────────────────

var docExtractToolDef = BaseTool{
	name: ToolDocExtract,
	description: `Extract structured key-value information from a piece of document text.

Use this tool when the user asks to extract:
- Named entities (people, organisations, locations, dates, amounts, contract terms, etc.)
- Specific fields they enumerate (e.g. "extract the name, date, and total amount")
- Any structured information embedded in unstructured text

Provide the document text in **content**.
Optionally list the field names you want extracted in **fields** (comma-separated).
If **fields** is omitted the model decides which fields are most relevant.

The tool returns a list of {name, value} pairs and an "output" summary suitable for the user.`,
	schema: utils.GenerateSchema[DocExtractInput](),
}

// DocExtractInput is the structured input for the doc_extract tool.
type DocExtractInput struct {
	Content string `json:"content" jsonschema:"required,Document text to extract information from"`
	Fields  string `json:"fields,omitempty" jsonschema:"Comma-separated list of field names to extract (optional)"`
}

// DocExtractTool extracts key-value pairs from document text.
type DocExtractTool struct {
	BaseTool
	chatModel chat.Chat
}

// NewDocExtractTool creates a new DocExtractTool.
func NewDocExtractTool(chatModel chat.Chat) *DocExtractTool {
	return &DocExtractTool{
		BaseTool:  docExtractToolDef,
		chatModel: chatModel,
	}
}

// Execute extracts fields from the provided content.
func (t *DocExtractTool) Execute(ctx context.Context, args json.RawMessage) (*types.ToolResult, error) {
	var input DocExtractInput
	if err := json.Unmarshal(args, &input); err != nil {
		return &types.ToolResult{Success: false, Error: fmt.Sprintf("invalid args: %v", err)}, err
	}
	if strings.TrimSpace(input.Content) == "" {
		return &types.ToolResult{Success: false, Error: "content is required"}, nil
	}

	fieldHint := "Extract all key information, entities, dates, amounts, names, and important facts."
	if input.Fields != "" {
		fieldHint = "Extract ONLY the following fields: " + input.Fields + "."
	}

	prompt := fmt.Sprintf(`You are a document information extraction specialist.
%s

Document content:
"""
%s
"""

Return ONLY valid JSON — no markdown, no extra text:
{
  "fields": [
    {"name": "<field name>", "value": "<extracted value>"},
    ...
  ]
}`, fieldHint, input.Content)

	thinking := false
	resp, err := t.chatModel.Chat(ctx, []chat.Message{
		{Role: "user", Content: prompt},
	}, &chat.ChatOptions{
		Temperature: 0.1,
		MaxTokens:   2048,
		Thinking:    &thinking,
	})
	if err != nil {
		logger.Errorf(ctx, "[Tool][DocExtract] LLM call failed: %v", err)
		return &types.ToolResult{Success: false, Error: fmt.Sprintf("LLM error: %v", err)}, err
	}

	raw := stripFence(resp.Content)
	var llmResp struct {
		Fields []map[string]string `json:"fields"`
	}
	if err := json.Unmarshal([]byte(raw), &llmResp); err != nil {
		logger.Warnf(ctx, "[Tool][DocExtract] non-JSON response, returning raw: %v", err)
		return &types.ToolResult{
			Success: true,
			Output:  resp.Content,
			Data: map[string]interface{}{
				"display_type": ToolDocExtract,
				"fields":       []map[string]string{{"name": "content", "value": resp.Content}},
			},
		}, nil
	}

	// Build human-readable output
	var sb strings.Builder
	sb.WriteString("Extracted fields:\n")
	for _, f := range llmResp.Fields {
		sb.WriteString(fmt.Sprintf("  %s: %s\n", f["name"], f["value"]))
	}

	return &types.ToolResult{
		Success: true,
		Output:  sb.String(),
		Data: map[string]interface{}{
			"display_type": ToolDocExtract,
			"fields":       llmResp.Fields,
		},
	}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Tool: doc_fill_table
// ─────────────────────────────────────────────────────────────────────────────

var docFillTableToolDef = BaseTool{
	name: ToolDocFillTable,
	description: `Fill in a table template by matching column headers to information from a source document.

Use this tool when the user wants to:
- Populate a form or table using information from a document passage
- Auto-fill contract fields, expense reports, registration forms, etc.
- Batch-fill multiple columns from a single information source

Provide:
- **source** – the document text containing the information (or a JSON list of {name,value} pairs from a prior doc_extract call)
- **columns** – comma-separated list of column/field names in the table template

The tool returns a filled table displayed inline in the chat with a CSV download option.`,
	schema: utils.GenerateSchema[DocFillTableInput](),
}

// DocFillTableInput is the structured input for the doc_fill_table tool.
type DocFillTableInput struct {
	Source  string `json:"source"  jsonschema:"required,Source document text or JSON extracted fields to draw values from"`
	Columns string `json:"columns" jsonschema:"required,Comma-separated list of column/field names in the table template"`
}

// DocFillTableTool fills a table template from source information.
type DocFillTableTool struct {
	BaseTool
	chatModel chat.Chat
}

// NewDocFillTableTool creates a new DocFillTableTool.
func NewDocFillTableTool(chatModel chat.Chat) *DocFillTableTool {
	return &DocFillTableTool{
		BaseTool:  docFillTableToolDef,
		chatModel: chatModel,
	}
}

// Execute fills the table columns from source information.
func (t *DocFillTableTool) Execute(ctx context.Context, args json.RawMessage) (*types.ToolResult, error) {
	var input DocFillTableInput
	if err := json.Unmarshal(args, &input); err != nil {
		return &types.ToolResult{Success: false, Error: fmt.Sprintf("invalid args: %v", err)}, err
	}
	if strings.TrimSpace(input.Source) == "" {
		return &types.ToolResult{Success: false, Error: "source is required"}, nil
	}
	if strings.TrimSpace(input.Columns) == "" {
		return &types.ToolResult{Success: false, Error: "columns is required"}, nil
	}

	// Split columns
	rawCols := strings.Split(input.Columns, ",")
	columns := make([]string, 0, len(rawCols))
	for _, c := range rawCols {
		c = strings.TrimSpace(c)
		if c != "" {
			columns = append(columns, c)
		}
	}
	if len(columns) == 0 {
		return &types.ToolResult{Success: false, Error: "no valid columns provided"}, nil
	}
	colList := strings.Join(columns, "\n")

	prompt := fmt.Sprintf(`You are a data entry assistant.
Fill in each column of the table using information from the source below.

SOURCE INFORMATION:
"""
%s
"""

TABLE COLUMNS (one per line):
%s

Rules:
- Match each column name to the most relevant piece of information in the source.
- If no matching information exists, use an empty string "".
- Self-assess your overall fill confidence on a 0-to-1 scale.

Return ONLY valid JSON — no markdown, no extra text:
{
  "filled_fields": [
    {"name": "<column name>", "value": "<filled value>"},
    ...
  ],
  "accuracy_hint": <float 0-1>
}`, input.Source, colList)

	thinking := false
	resp, err := t.chatModel.Chat(ctx, []chat.Message{
		{Role: "user", Content: prompt},
	}, &chat.ChatOptions{
		Temperature: 0.1,
		MaxTokens:   2048,
		Thinking:    &thinking,
	})
	if err != nil {
		logger.Errorf(ctx, "[Tool][DocFillTable] LLM call failed: %v", err)
		return &types.ToolResult{Success: false, Error: fmt.Sprintf("LLM error: %v", err)}, err
	}

	raw := stripFence(resp.Content)
	var llmResp struct {
		FilledFields []map[string]string `json:"filled_fields"`
		AccuracyHint float64             `json:"accuracy_hint"`
	}
	if err := json.Unmarshal([]byte(raw), &llmResp); err != nil {
		logger.Warnf(ctx, "[Tool][DocFillTable] non-JSON response: %v", err)
		// Fallback: one row with empty values
		for _, col := range columns {
			llmResp.FilledFields = append(llmResp.FilledFields, map[string]string{"name": col, "value": ""})
		}
		llmResp.AccuracyHint = 0
	}

	// Clamp accuracy
	if llmResp.AccuracyHint < 0 {
		llmResp.AccuracyHint = 0
	} else if llmResp.AccuracyHint > 1 {
		llmResp.AccuracyHint = 1
	}

	// Build CSV string for download
	csvContent := buildCSVFromFields(llmResp.FilledFields)

	// Build human-readable output
	var sb strings.Builder
	sb.WriteString("Table filled successfully:\n\n")
	for _, f := range llmResp.FilledFields {
		sb.WriteString(fmt.Sprintf("  %s: %s\n", f["name"], f["value"]))
	}
	sb.WriteString(fmt.Sprintf("\nEstimated accuracy: %.0f%%", llmResp.AccuracyHint*100))

	return &types.ToolResult{
		Success: true,
		Output:  sb.String(),
		Data: map[string]interface{}{
			"display_type":  ToolDocFillTable,
			"filled_fields": llmResp.FilledFields,
			"accuracy_hint": llmResp.AccuracyHint,
			"csv_content":   csvContent,
			"columns":       columns,
		},
	}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers shared by all three tools
// ─────────────────────────────────────────────────────────────────────────────

// stripFence removes the ```json ... ``` or ``` ... ``` wrapper that some models add.
func stripFence(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		lines := strings.SplitN(s, "\n", 2)
		if len(lines) == 2 {
			s = lines[1]
		}
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	}
	return strings.TrimSpace(s)
}

// buildCSVFromFields produces a two-row CSV (header + values).
func buildCSVFromFields(fields []map[string]string) string {
	if len(fields) == 0 {
		return ""
	}
	// Escape a single CSV field: wrap in quotes if it contains comma, newline, or quote.
	escape := func(v string) string {
		if strings.ContainsAny(v, ",\"\n\r") {
			v = strings.ReplaceAll(v, `"`, `""`)
			return `"` + v + `"`
		}
		return v
	}
	headers := make([]string, len(fields))
	values := make([]string, len(fields))
	for i, f := range fields {
		headers[i] = escape(f["name"])
		values[i] = escape(f["value"])
	}
	return strings.Join(headers, ",") + "\n" + strings.Join(values, ",") + "\n"
}
