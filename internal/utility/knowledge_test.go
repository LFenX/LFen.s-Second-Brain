package utility

import "testing"

func TestKnowledgeTextSourceAllowlist(t *testing.T) {
	for _, name := range []string{"demo.tsx", "demo.jsx", "demo.css", "demo.scss", "demo.go", "demo.rs", "demo.yaml", "note.md"} {
		if !IsKnowledgeTextSource(name, []byte("标题\r\ncode\n")) {
			t.Fatalf("rejected %s", name)
		}
	}
	for _, tc := range []struct {
		name string
		body []byte
	}{{"slides.pptx", []byte("text")}, {"paper.pdf", []byte("text")}, {"bad.tsx", []byte{0xff}}, {"bad.md", []byte{0}}, {"binary.exe", []byte("text")}} {
		if IsKnowledgeTextSource(tc.name, tc.body) {
			t.Fatalf("accepted unsupported source %s", tc.name)
		}
	}
}
