package utility

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestKnowledgeDirectoryPaths(t *testing.T) {
	for _, p := range []string{"web/card.tsx", "docs/中文.md", "a%2fb/file.ts", "a/b/c.go"} {
		if !IsKnowledgeDirectoryPath(p, false) {
			t.Errorf("valid path rejected: %s", p)
		}
	}
	for _, p := range []string{"", ".", "../x.ts", "a/../x.ts", "/tmp/x.ts", "Z:/x.ts", "a\\x.ts", "a//x.ts", "a/*.ts", "a/.env.local", "a/node_modules/x.ts", "a/CON.ts", "a/a./x.ts", "a/id_rsa", "a/secret.pem", "a/ x.ts", "a\x00/b.ts"} {
		if IsKnowledgeDirectoryPath(p, false) {
			t.Errorf("invalid path accepted: %q", p)
		}
	}
	if !IsKnowledgeDirectoryPath(".", true) {
		t.Fatal("directory root rejected")
	}
}
func TestKnowledgeDirectoryCapabilityAndLinks(t *testing.T) {
	rootPath := t.TempDir()
	if !KnowledgeDirectoryPlatformVerified() {
		if root, _, err := OpenKnowledgeDirectoryRoot(rootPath); err == nil {
			root.Close()
			t.Fatal("unverified platform opened root")
		}
		return
	}
	content := []byte("\ufeff// 合成\r\n你好 🧠\r\n")
	file := filepath.Join(rootPath, "a.ts")
	if err := os.WriteFile(file, content, 0600); err != nil {
		t.Fatal(err)
	}
	root, identity, err := OpenKnowledgeDirectoryRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if identity == "" {
		t.Fatal("empty identity")
	}
	got, _, err := ReadKnowledgeDirectoryFile(t.Context(), root, "a.ts", 2097152)
	if err != nil || string(got) != string(content) {
		t.Fatalf("read=%q err=%v", got, err)
	}
	if err := os.Symlink(file, filepath.Join(rootPath, "link.ts")); err != nil {
		t.Fatal(err)
	}
	if _, _, err = ReadKnowledgeDirectoryFile(t.Context(), root, "link.ts", 2097152); err == nil {
		t.Fatal("symlink accepted")
	}
	if err := os.Link(file, filepath.Join(rootPath, "hard.ts")); err != nil {
		t.Fatal(err)
	}
	if _, _, err = ReadKnowledgeDirectoryFile(t.Context(), root, "hard.ts", 2097152); err == nil {
		t.Fatal("hardlink accepted")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err = ReadKnowledgeDirectoryFile(ctx, root, "a.ts", 2097152); err == nil {
		t.Fatal("cancel ignored")
	}
}
