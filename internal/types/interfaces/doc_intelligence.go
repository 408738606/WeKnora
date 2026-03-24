package interfaces

import (
	"context"
	"mime/multipart"

	"github.com/Tencent/WeKnora/internal/types"
)

// DocIntelligenceService exposes the three document intelligence modules:
//
//  1. Natural language document operation (parse instruction → execute on content)
//  2. Unstructured document information extraction + structured storage
//  3. Table template filling using previously extracted data
type DocIntelligenceService interface {
	// ─── Module 1: Natural Language Document Operation ───────────────────────

	// ExecuteInstruction parses a natural-language instruction and applies it to
	// the supplied document content (or to the parsed text of fileHeader when
	// docContent is empty).  The result is persisted as a DocInstructionLog.
	ExecuteInstruction(
		ctx context.Context,
		req *types.DocInstructionRequest,
		fileHeader *multipart.FileHeader,
	) (*types.DocInstructionResult, error)

	// ─── Module 2: Document Information Extraction ───────────────────────────

	// ExtractFromFile parses fileHeader, uses an LLM to extract key fields
	// (optionally filtered by opts.Fields), and persists an ExtractedData record.
	ExtractFromFile(
		ctx context.Context,
		fileHeader *multipart.FileHeader,
		opts *types.DocExtractOptions,
	) (*types.ExtractedDataResponse, error)

	// ListExtractedData returns all ExtractedData records for the current tenant.
	ListExtractedData(ctx context.Context) ([]*types.ExtractedData, error)

	// GetExtractedData returns a single ExtractedData record by ID.
	GetExtractedData(ctx context.Context, id string) (*types.ExtractedData, error)

	// DeleteExtractedData soft-deletes an ExtractedData record.
	DeleteExtractedData(ctx context.Context, id string) error

	// ─── Module 3: Table Custom Data Filling ─────────────────────────────────

	// FillTable parses the table template (Excel/Word/CSV) from templateHeader,
	// sources field values from a prior extraction record or inline docContent,
	// and returns a filled TableFillResult including a CSV representation.
	FillTable(
		ctx context.Context,
		templateHeader *multipart.FileHeader,
		opts *types.TableFillOptions,
	) (*types.TableFillResult, error)
}
