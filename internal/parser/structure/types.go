// Package structure produces deterministic source ranges and conservative static
// facts from explicitly supplied UTF-8 files. It performs no I/O and stores no
// second copy of the source text. Authorization and view identities belong to the
// caller; local keys are rebound to the caller's fixed view after production.
package structure

import "fmt"

const (
	MaxFiles        = 32
	MaxFileBytes    = 2 << 20
	MaxInputBytes   = 16 << 20
	MaxNodesPerFile = 8192
	MaxNodes        = 32768
	MaxRelations    = 65536
	MaxDepth        = 16
	PassageTarget   = 4096
	PassageMax      = 8192
)

type Input struct {
	SourceID   string
	RevisionID string
	Path       string
	Parser     string
	Content    []byte
}

// Alias has the same prefix/target field names as the frozen public request.
type Alias struct {
	Prefix string `json:"prefix"`
	Target string `json:"target"`
}

type Output struct {
	Files []File
}

type File struct {
	SourceID    string
	RevisionID  string
	Path        string
	Parser      string
	SHA256      string
	ByteLength  int
	Nodes       []Node
	Relations   []Relation
	Diagnostics []Diagnostic
}

type Node struct {
	Key               string
	ParentKey         string // Empty only for the file root.
	Kind              string // file, symbol, section, passage.
	Name              string
	LogicalKey        string
	IdentityStability string // file, name_scope, position_based, range_based.
	StartByte         int
	EndByte           int
	StartLine         int
	EndLine           int
}

type Ref struct {
	SourceID string
	Key      string
}

type Relation struct {
	Kind               string
	From               Ref
	To                 *Ref
	Resolution         string
	Reason             string
	Reference          string
	ReferenceTruncated bool
	Evidence           Evidence
}

type Evidence struct {
	SourceID       string
	RevisionID     string
	StartByte      int
	EndByte        int
	FragmentSHA256 string
}

type Diagnostic struct {
	Code      string
	Message   string
	StartByte *int
	EndByte   *int
}

// Error is deliberately controlled: it never includes source text or a raw
// parser diagnostic. A failed build returns no publishable partial Output.
type Error struct {
	Code     string
	SourceID string
}

func (e *Error) Error() string {
	if e.SourceID == "" {
		return "structure: " + e.Code
	}
	return fmt.Sprintf("structure: %s (source %s)", e.Code, e.SourceID)
}
