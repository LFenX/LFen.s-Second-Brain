package entity

import "encoding/json"

// KnowledgeDocumentLocator uses custom JSON to emit exactly one frozen scheme.
type KnowledgeDocumentLocator struct {
	Scheme        string
	PageNumber    int64
	Part          string
	Segments      []KnowledgeDocumentOfficeSegment
	ParentLocator *KnowledgeDocumentLocator
	ChunkIndex    int64
}
type KnowledgeDocumentOfficeSegment struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Index     int64  `json:"index"`
}
type KnowledgeDocumentNaturalRef struct {
	Kind    string                   `json:"kind"`
	Locator KnowledgeDocumentLocator `json:"locator"`
}
type KnowledgeDocumentOriginalPosition struct {
	Kind        string
	PageNumber  int64
	SlideNumber int64
}
type KnowledgeDocumentReadingRange struct {
	StartByte  int64  `json:"start_byte"`
	EndByte    int64  `json:"end_byte"`
	TextSHA256 string `json:"text_sha256"`
}

// KnowledgeDocumentNodePayload contains the natural preimage, before derived IDs.
// All nullable fields are required JSON keys; none use omitempty.
type KnowledgeDocumentNodePayload struct {
	Kind                string                            `json:"kind"`
	Attributes          json.RawMessage                   `json:"attributes"`
	Locator             KnowledgeDocumentLocator          `json:"locator"`
	ParentRef           *KnowledgeDocumentNaturalRef      `json:"parent_ref"`
	Ordinal             int64                             `json:"ordinal"`
	Name                string                            `json:"name"`
	NameTruncated       bool                              `json:"name_truncated"`
	OriginalPosition    KnowledgeDocumentOriginalPosition `json:"original_position"`
	CoordinatePrecision string                            `json:"coordinate_precision"`
	TextStatus          string                            `json:"text_status"`
	TextReason          *string                           `json:"text_reason"`
	ReadingRange        KnowledgeDocumentReadingRange     `json:"reading_range"`
	OwnReadingRange     *KnowledgeDocumentReadingRange    `json:"own_reading_range"`
}
type KnowledgeDocumentEmptyAttributes struct{}
type KnowledgeDocumentPageAttributes struct {
	PageNumber                int64  `json:"page_number"`
	PageContentClassification string `json:"page_content_classification"`
}
type KnowledgeDocumentSlideAttributes struct {
	SlideNumber int64 `json:"slide_number"`
	Hidden      bool  `json:"hidden"`
}
type KnowledgeDocumentHeadingAttributes struct {
	Level *int64 `json:"level"`
}
type KnowledgeDocumentTableAttributes struct {
	RowCount        int64  `json:"row_count"`
	MaxCellsPerRow  int64  `json:"max_cells_per_row"`
	LayoutPrecision string `json:"layout_precision"`
}
type KnowledgeDocumentRowAttributes struct {
	RowIndex  int64 `json:"row_index"`
	CellCount int64 `json:"cell_count"`
}
type KnowledgeDocumentCellAttributes struct {
	RowIndex   int64  `json:"row_index"`
	CellIndex  int64  `json:"cell_index"`
	RowSpan    *int64 `json:"row_span"`
	ColumnSpan *int64 `json:"column_span"`
	IsHeader   *bool  `json:"is_header"`
}
type KnowledgeDocumentListAttributes struct {
	Ordered      *bool   `json:"ordered"`
	StartNumber  *int64  `json:"start_number"`
	SourceLevel  *int64  `json:"source_level"`
	NumberingID  *string `json:"numbering_id"`
	NumberFormat *string `json:"number_format"`
}
type KnowledgeDocumentListItemAttributes struct {
	ItemIndex int64 `json:"item_index"`
}
type KnowledgeDocumentMediaAttributes struct {
	Placement        string  `json:"placement"`
	InlineAnchorByte *int64  `json:"inline_anchor_byte"`
	MediaType        *string `json:"media_type"`
	ByteLength       *int64  `json:"byte_length"`
	Origin           string  `json:"origin"`
	OCRStatus        string  `json:"ocr_status"`
}
type KnowledgeDocumentPassageAttributes struct {
	ChunkIndex int64 `json:"chunk_index"`
}
type KnowledgeDocumentUnsupportedBlockAttributes struct {
	NativeType string `json:"native_type"`
	Reason     string `json:"reason"`
}
type KnowledgeDocumentManifestEntry struct {
	NaturalRef        KnowledgeDocumentNaturalRef `json:"natural_ref"`
	NodePayloadSHA256 string                      `json:"node_payload_sha256"`
}
type KnowledgeDocumentStructureManifest struct {
	Nodes []KnowledgeDocumentManifestEntry `json:"nodes"`
}
type KnowledgeDocumentNode struct {
	RepresentationID  string                       `json:"representation_id"`
	DocumentNodeID    string                       `json:"document_node_id"`
	SourceID          string                       `json:"source_id"`
	SourceRevisionID  string                       `json:"source_revision_id"`
	OriginalSHA256    string                       `json:"original_sha256"`
	ProducerSHA256    string                       `json:"producer_sha256"`
	ParentID          *string                      `json:"parent_id"`
	NodePayloadSHA256 string                       `json:"node_payload_sha256"`
	Payload           KnowledgeDocumentNodePayload `json:"payload"`
}
