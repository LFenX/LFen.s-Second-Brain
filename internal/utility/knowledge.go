package utility

import (
	"bytes"
	"path"
	"strings"
	"unicode/utf8"
)

// IsKnowledgeTextSource is the shared allowlist for catalogue upload and snapshot.
// It recognizes source text; it does not parse documents or infer semantics.
func IsKnowledgeTextSource(name string, content []byte) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".txt", ".md", ".markdown", ".ts", ".tsx", ".js", ".jsx", ".go", ".css", ".scss", ".json", ".yaml", ".yml", ".html", ".htm", ".sql", ".sh", ".c", ".cpp", ".h", ".hpp", ".java", ".rs", ".py":
		return utf8.Valid(content) && !bytes.ContainsRune(content, 0)
	}
	return false
}
