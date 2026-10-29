package entity

// KnowledgeContentVersion identifies the unified projection, not a new source store.
const KnowledgeContentVersion = "0.2.0-candidate"

type KnowledgeContentSourceRef struct {
	DomainID   string `json:"domain_id"`
	SourceID   string `json:"source_id"`
	RevisionID string `json:"source_revision_id"`
	SHA256     string `json:"source_sha256"`
}

type KnowledgeContentRepresentationRef struct {
	Kind           string  `json:"kind"`
	ID             *string `json:"id"`
	ProducerSHA256 *string `json:"producer_sha256"`
}

// A target is always bound. Unbound parser output has a representation but no target.
type KnowledgeContentTarget struct {
	Source         KnowledgeContentSourceRef         `json:"source_ref"`
	Representation KnowledgeContentRepresentationRef `json:"representation_ref"`
}

type KnowledgeContentNodeRef struct {
	Kind      string  `json:"kind"`
	ID        *string `json:"id"`
	ItemIndex *int64  `json:"item_index"`
}

type KnowledgeContentAddress struct {
	Target KnowledgeContentTarget  `json:"target"`
	Node   KnowledgeContentNodeRef `json:"node_ref"`
}

type KnowledgeContentRange struct {
	StartByte int64   `json:"start_byte"`
	EndByte   int64   `json:"end_byte"`
	SHA256    *string `json:"sha256"`
}

type KnowledgeContentSourceLocator struct {
	StartByte int64 `json:"start_byte"`
	EndByte   int64 `json:"end_byte"`
	StartLine int64 `json:"start_line"`
	EndLine   int64 `json:"end_line"`
}

// Exactly one arm is populated. The existing document locator keeps its own scheme.
type KnowledgeContentLocator struct {
	Kind       string                         `json:"kind"`
	SourceUTF8 *KnowledgeContentSourceLocator `json:"source_utf8"`
	Document   *KnowledgeDocumentLocator      `json:"document"`
	ItemIndex  *int64                         `json:"item_index"`
}

type KnowledgeContentText struct {
	CoordinateSpace string                 `json:"coordinate_space"`
	Range           *KnowledgeContentRange `json:"range"`
	OwnRange        *KnowledgeContentRange `json:"own_range"`
	Status          string                 `json:"status"`
	Reason          *string                `json:"reason"`
	Value           *string                `json:"value"`
}

type KnowledgeContentSourceAttributes struct {
	LogicalID        string `json:"logical_id"`
	LogicalKey       string `json:"logical_key"`
	StructureVersion string `json:"structure_version"`
}

type KnowledgeContentStructureAttributes struct {
	LogicalNodeID     string                           `json:"logical_node_id"`
	LogicalKey        string                           `json:"logical_key"`
	IdentityStability string                           `json:"identity_stability"`
	Parser            string                           `json:"parser"`
	Classification    KnowledgeStructureClassification `json:"classification"`
}

// Document attributes reuse the existing owning types and contain one kind arm.
type KnowledgeContentDocumentAttributes struct {
	Empty            *KnowledgeDocumentEmptyAttributes            `json:"empty"`
	Page             *KnowledgeDocumentPageAttributes             `json:"page"`
	Slide            *KnowledgeDocumentSlideAttributes            `json:"slide"`
	Heading          *KnowledgeDocumentHeadingAttributes          `json:"heading"`
	Table            *KnowledgeDocumentTableAttributes            `json:"table"`
	Row              *KnowledgeDocumentRowAttributes              `json:"row"`
	Cell             *KnowledgeDocumentCellAttributes             `json:"cell"`
	List             *KnowledgeDocumentListAttributes             `json:"list"`
	ListItem         *KnowledgeDocumentListItemAttributes         `json:"list_item"`
	Media            *KnowledgeDocumentMediaAttributes            `json:"media"`
	Passage          *KnowledgeDocumentPassageAttributes          `json:"passage"`
	UnsupportedBlock *KnowledgeDocumentUnsupportedBlockAttributes `json:"unsupported_block"`
}

// Positions are retained as unbound producer values; they do not assert a page frame.
type KnowledgeContentParserAttributes struct {
	ItemType       string      `json:"item_type"`
	CKType         string      `json:"ck_type"`
	Table          *TableData  `json:"table"`
	Sheet          string      `json:"sheet"`
	Positions      [][]float64 `json:"positions"`
	AltText        *string     `json:"alt_text"`
	PayloadPresent bool        `json:"payload_present"`
	TextAssembly   *string     `json:"text_assembly"`
}

type KnowledgeContentAttributes struct {
	Source    *KnowledgeContentSourceAttributes    `json:"source"`
	Structure *KnowledgeContentStructureAttributes `json:"structure"`
	Document  *KnowledgeContentDocumentAttributes  `json:"document"`
	Parser    *KnowledgeContentParserAttributes    `json:"parser"`
}

type KnowledgeContentNode struct {
	Ref                 KnowledgeContentNodeRef            `json:"ref"`
	ParentRef           *KnowledgeContentNodeRef           `json:"parent_ref"`
	Kind                string                             `json:"kind"`
	Name                string                             `json:"name"`
	NameTruncated       bool                               `json:"name_truncated"`
	Ordinal             *int64                             `json:"ordinal"`
	Locator             KnowledgeContentLocator            `json:"locator"`
	OriginalPosition    *KnowledgeDocumentOriginalPosition `json:"original_position"`
	CoordinatePrecision string                             `json:"coordinate_precision"`
	Text                KnowledgeContentText               `json:"text"`
	Attributes          KnowledgeContentAttributes         `json:"attributes"`
	AttachmentRef       *KnowledgeContentSourceRef         `json:"attachment_ref"`
}

type KnowledgeContentRelationEvidence struct {
	Source KnowledgeContentSourceRef `json:"source_ref"`
	Range  KnowledgeContentRange     `json:"range"`
}

type KnowledgeContentRelationship struct {
	ID                 string                           `json:"id"`
	Kind               string                           `json:"kind"`
	From               KnowledgeContentAddress          `json:"from"`
	To                 *KnowledgeContentAddress         `json:"to"`
	Resolution         string                           `json:"resolution"`
	Reason             string                           `json:"reason"`
	Reference          string                           `json:"reference"`
	ReferenceTruncated bool                             `json:"reference_truncated"`
	Evidence           KnowledgeContentRelationEvidence `json:"evidence"`
}

type KnowledgeContentRepresentation struct {
	Version         string                            `json:"projection_version"`
	Binding         string                            `json:"binding"`
	Source          *KnowledgeContentSourceRef        `json:"source_ref"`
	Ref             KnowledgeContentRepresentationRef `json:"representation_ref"`
	RootRef         *KnowledgeContentNodeRef          `json:"root_ref"`
	State           string                            `json:"state"`
	Reason          *string                           `json:"reason"`
	CoordinateSpace string                            `json:"coordinate_space"`
	NodeCount       *int64                            `json:"node_count"`
}

type KnowledgeContentProjection struct {
	KnowledgeContentRepresentation
	Nodes         []KnowledgeContentNode         `json:"nodes"`
	Relationships []KnowledgeContentRelationship `json:"relationships"`
}

type KnowledgeContentNodeResult struct {
	Version string                 `json:"projection_version"`
	Target  KnowledgeContentTarget `json:"target"`
	Node    KnowledgeContentNode   `json:"node"`
}

type KnowledgeContentNodeQuery struct {
	Target KnowledgeContentTarget   `json:"target"`
	Parent *KnowledgeContentNodeRef `json:"parent_ref"`
	Cursor string                   `json:"cursor"`
	Limit  int                      `json:"limit"`
}

type KnowledgeContentNodePage struct {
	Version    string                   `json:"projection_version"`
	Target     KnowledgeContentTarget   `json:"target"`
	Parent     *KnowledgeContentNodeRef `json:"parent_ref"`
	Items      []KnowledgeContentNode   `json:"items"`
	NextCursor *string                  `json:"next_cursor"`
}

type KnowledgeContentReadRequest struct {
	Address   KnowledgeContentAddress `json:"address"`
	StartByte *int64                  `json:"start_byte"`
	EndByte   *int64                  `json:"end_byte"`
}

type KnowledgeContentWindow struct {
	Version         string                  `json:"projection_version"`
	Address         KnowledgeContentAddress `json:"address"`
	Locator         KnowledgeContentLocator `json:"locator"`
	CoordinateSpace string                  `json:"coordinate_space"`
	Range           KnowledgeContentRange   `json:"range"`
	TextStatus      string                  `json:"text_status"`
	TextReason      *string                 `json:"text_reason"`
	Text            string                  `json:"text"`
	Complete        bool                    `json:"complete"`
	NextStartByte   *int64                  `json:"next_start_byte"`
}
