package tokenizer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestKnowledgeLocalLexicalProducerTracksExecutableAndResources(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "synthetic-executable")
	if err := os.WriteFile(exe, []byte("synthetic native implementation A"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"rag", "wordnet", "opencc"} {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name, "synthetic.dict"), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	a, err := fingerprintLexicalFiles(exe, root)
	if err != nil {
		t.Fatal(err)
	}
	again, err := fingerprintLexicalFiles(exe, root)
	if err != nil || again != a {
		t.Fatalf("not repeatable: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "rag", "synthetic.dict"), []byte("changed synthetic dictionary"), 0600); err != nil {
		t.Fatal(err)
	}
	b, err := fingerprintLexicalFiles(exe, root)
	if err != nil || a == b {
		t.Fatal("resource drift unbound")
	}
	if err := os.WriteFile(exe, []byte("synthetic native implementation B"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := fingerprintLexicalFiles(exe, root)
	if err != nil || b == c {
		t.Fatal("native implementation drift unbound")
	}
}
