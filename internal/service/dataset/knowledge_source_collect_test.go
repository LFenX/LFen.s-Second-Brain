package dataset

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"ragflow/internal/entity"
)

func preparationOptions() knowledgeCollectionOptions {
	return knowledgeCollectionOptions{RegistrationKey: "synthetic-project", IncludePaths: []string{"."}, MaxFiles: 20000, MaxFileBytes: 8 << 20,
		Rules: []knowledgeSourceRule{{Prefix: ".", ComponentSide: "non_code"}, {Prefix: "web", ComponentSide: "frontend", Language: "typescript"}, {Prefix: "web/shared", ComponentSide: "shared", Language: "typescript"}, {Prefix: "internal", ComponentSide: "backend", Language: "go"}, {Prefix: "cmd", ComponentSide: "cli", Language: "go"}}}
}

func writePreparationSource(t testing.TB, root, name string, data []byte) {
	t.Helper()
	file := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func requirePreparationManifest(t testing.TB, root string, options knowledgeCollectionOptions) *knowledgeSourceManifest {
	t.Helper()
	manifest, err := collectKnowledgeSources(context.Background(), root, options)
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}

func TestKnowledgeSourcePreparationCollection(t *testing.T) {
	root := t.TempDir()
	fixtures := map[string][]byte{
		"web/table.tsx":       []byte("\xef\xbb\xbfexport const title = '合成表格🧠';\r\n"),
		"web/shared/types.ts": []byte("export type Row = {id: string};\n"),
		"internal/table.go":   []byte("package synthetic\n"),
		"cmd/tool.go":         []byte("package main\n"),
		"notes/empty.md":      {}, "notes/duplicate.md": []byte("package synthetic\n"),
		"notes/doc.pdf": []byte("%PDF-1.4\x00synthetic"), "notes/picture.bin": {0, 255, 17},
		".env.local": []byte("SYNTHETIC=excluded"), "node_modules/a.ts": []byte("excluded"),
		"dist/out.js": []byte("excluded"), "notes/key.pem": []byte("excluded"),
	}
	for name, data := range fixtures {
		writePreparationSource(t, root, name, data)
	}
	options := preparationOptions()
	first := requirePreparationManifest(t, root, options)
	if !first.Complete || len(first.Items) != 8 || len(first.Excluded) != 4 {
		t.Fatalf("unexpected manifest: %+v", first)
	}
	for _, item := range first.Items {
		digest := sha256.Sum256(fixtures[item.Path])
		if item.State != "observed" || item.ByteLength != int64(len(fixtures[item.Path])) || item.SHA256 != hex.EncodeToString(digest[:]) {
			t.Fatalf("bytes changed: %+v", item)
		}
		expected := map[string]string{"web/table.tsx": "frontend", "web/shared/types.ts": "shared", "internal/table.go": "backend", "cmd/tool.go": "cli"}
		side := expected[item.Path]
		if side == "" {
			side = "non_code"
		}
		if item.ComponentSide != side || item.C2Eligible != (item.Kind == "text") {
			t.Fatalf("classification changed: %+v", item)
		}
	}
	for _, item := range first.Excluded {
		if item.Reason != "excluded_path" {
			t.Fatalf("excluded item reached observation: %+v", item)
		}
	}
	// Host paths, input order, and modification timestamps do not affect identity.
	other := t.TempDir()
	for name, data := range fixtures {
		writePreparationSource(t, other, name, data)
	}
	options.Rules[0], options.Rules[1] = options.Rules[1], options.Rules[0]
	second := requirePreparationManifest(t, other, options)
	if first.SHA256 != second.SHA256 {
		t.Fatal("same bytes and rules on a different root changed the manifest")
	}
	if err := verifyKnowledgeCollection(t.Context(), root, preparationOptions(), first); err != nil {
		t.Fatal(err)
	}
	writePreparationSource(t, root, "web/table.tsx", []byte("changed after initial collection"))
	if err := verifyKnowledgeCollection(t.Context(), root, preparationOptions(), first); err == nil {
		t.Fatal("stale manifest admitted changed source bytes")
	}
}

func TestKnowledgeSourcePreparationScopeAndBounds(t *testing.T) {
	t.Run("precise selection", func(t *testing.T) {
		root := t.TempDir()
		writePreparationSource(t, root, "web/a.ts", []byte("selected"))
		writePreparationSource(t, root, "web-other/a.ts", []byte("outside"))
		writePreparationSource(t, root, "internal/a.go", []byte("outside"))
		options := preparationOptions()
		options.IncludePaths = []string{"web"}
		manifest := requirePreparationManifest(t, root, options)
		if !manifest.Complete || len(manifest.Items) != 1 || manifest.Items[0].Path != "web/a.ts" || len(manifest.Excluded) != 2 {
			t.Fatalf("selection leaked: %+v", manifest)
		}
	})
	for _, selection := range []string{"../outside", "/absolute", "C:/private", "web\\a.ts", "web//a.ts", ".env.local", "node_modules", "web/../internal"} {
		t.Run(selection, func(t *testing.T) {
			options := preparationOptions()
			options.IncludePaths = []string{selection}
			if _, err := collectKnowledgeSources(t.Context(), t.TempDir(), options); err == nil {
				t.Fatal("invalid selection accepted")
			}
		})
	}
	t.Run("count and cancellation", func(t *testing.T) {
		root := t.TempDir()
		writePreparationSource(t, root, "a.md", []byte("a"))
		writePreparationSource(t, root, "b.md", []byte("b"))
		options := preparationOptions()
		options.MaxFiles = 1
		if _, err := collectKnowledgeSources(t.Context(), root, options); err == nil {
			t.Fatal("file count limit ignored")
		}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := collectKnowledgeSources(ctx, root, preparationOptions()); err == nil {
			t.Fatal("cancelled collector returned a complete snapshot")
		}
	})
	t.Run("size utf8 and secret boundaries", func(t *testing.T) {
		root := t.TempDir()
		cases := map[string]struct {
			data     []byte
			state    string
			eligible bool
		}{
			"large.ts":  {bytes.Repeat([]byte("a"), int(entity.KnowledgeMaxSourceBytes)+1), "observed", false},
			"bad.ts":    {[]byte{0xff, 'a'}, "invalid_text", false},
			"nul.md":    {[]byte{'a', 0, 'b'}, "invalid_text", false},
			"secret.md": {[]byte(strings.Repeat("a", 32760) + "-----BEGIN PRIVATE KEY-----"), "secret_marker", false},
			"rune.md":   {[]byte(strings.Repeat("a", 32767) + "🧠中"), "observed", true},
		}
		for name, item := range cases {
			writePreparationSource(t, root, name, item.data)
		}
		manifest := requirePreparationManifest(t, root, preparationOptions())
		if manifest.Complete {
			t.Fatal("blocked files yielded an admissible manifest")
		}
		for _, item := range manifest.Items {
			want := cases[item.Path]
			if item.State != want.state || item.C2Eligible != want.eligible || (item.State != "observed" && item.SHA256 != "") {
				t.Fatalf("wrong boundary result: %+v", item)
			}
		}
		options := preparationOptions()
		options.MaxFileBytes = 16
		oversize := requirePreparationManifest(t, root, options)
		for _, item := range oversize.Items {
			if int64(len(cases[item.Path].data)) > 16 && (item.State != "too_large" || item.SHA256 != "") {
				t.Fatalf("large file was truncated or hashed: %+v", item)
			}
		}
	})
	t.Run("missing selection", func(t *testing.T) {
		options := preparationOptions()
		options.IncludePaths = []string{"missing"}
		manifest := requirePreparationManifest(t, t.TempDir(), options)
		if manifest.Complete || len(manifest.Excluded) != 1 || manifest.Excluded[0].Reason != "missing_selection" {
			t.Fatalf("missing scope hidden: %+v", manifest)
		}
	})
}

func TestKnowledgeSourcePreparationLinks(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	writePreparationSource(t, outside, "private.md", []byte("outside must never enter the manifest"))
	writePreparationSource(t, root, "inside.md", []byte("inside"))
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatalf("symlink test environment unavailable: %v", err)
	}
	if err := os.Symlink(filepath.Join(outside, "private.md"), filepath.Join(root, "escape.md")); err != nil {
		t.Fatal(err)
	}
	manifest := requirePreparationManifest(t, root, preparationOptions())
	if manifest.Complete || len(manifest.Items) != 1 || manifest.Items[0].Path != "inside.md" || len(manifest.Excluded) != 2 {
		t.Fatalf("link boundary failed: %+v", manifest)
	}
	for _, item := range manifest.Excluded {
		if item.Reason != "link" {
			t.Fatalf("link entered observation: %+v", item)
		}
	}
	if _, err := collectKnowledgeSources(t.Context(), filepath.Join(root, "escape"), preparationOptions()); err == nil {
		t.Fatal("link root accepted")
	}
	// Swap a selected file after enumeration. Root confines the actual open even
	// if a caller kept a stale path; no hash of the outside bytes is emitted.
	r, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := os.Remove(filepath.Join(root, "inside.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "private.md"), filepath.Join(root, "inside.md")); err != nil {
		t.Fatal(err)
	}
	item := observeKnowledgeSource(t.Context(), r, "inside.md", 1024, make([]byte, 64))
	if item.State == "observed" || item.SHA256 != "" {
		t.Fatalf("replaced path followed outside the root: %+v", item)
	}
}

func TestKnowledgeSourcePreparationStreamingProbe(t *testing.T) {
	for _, input := range [][]byte{[]byte("中文🧠\r\n"), []byte("\xef\xbb\xbf"), {}, {0xf0, 0x9f}, {0xff}, {'x', 0, 'y'}, []byte("-----BEGIN RSA PRIVATE KEY-----")} {
		probe := &knowledgeSourceProbe{valid: true}
		for _, value := range input {
			_, _ = probe.Write([]byte{value})
		}
		if (probe.valid && len(probe.pending) == 0) != bytes.Equal([]byte(strings.ToValidUTF8(string(input), "?")), input) {
			t.Fatalf("streaming UTF8 mismatch: %x", input)
		}
		if probe.nul != bytes.Contains(input, []byte{0}) || probe.secret != bytes.Contains(input, []byte("-----BEGIN RSA PRIVATE KEY-----")) {
			t.Fatalf("streaming boundary marker mismatch: %x", input)
		}
	}
}

// The context reader is the existing I/O boundary. Replace the directory entry
// between two reads without adding a hook or global fault switch to production.
type preparationMutationContext struct {
	context.Context
	reads  int
	once   sync.Once
	mutate func()
}

func (c *preparationMutationContext) Err() error {
	c.reads++
	if c.reads == 2 {
		c.once.Do(c.mutate)
	}
	return c.Context.Err()
}

func TestKnowledgeSourcePreparationChangedDuringRead(t *testing.T) {
	root := t.TempDir()
	writePreparationSource(t, root, "source.ts", bytes.Repeat([]byte("a"), 128*1024))
	writePreparationSource(t, root, "replacement.ts", bytes.Repeat([]byte("b"), 128*1024))
	r, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var mutationErr error
	ctx := &preparationMutationContext{Context: t.Context(), mutate: func() {
		mutationErr = os.Rename(filepath.Join(root, "replacement.ts"), filepath.Join(root, "source.ts"))
	}}
	item := observeKnowledgeSource(ctx, r, "source.ts", 8<<20, make([]byte, 32768))
	if mutationErr != nil {
		t.Fatal(mutationErr)
	}
	if ctx.reads < 2 || item.State != "changed_during_read" || item.SHA256 != "" || item.C2Eligible {
		t.Fatalf("mid-read replacement admitted: %+v", item)
	}
}

func BenchmarkKnowledgeSourcePreparationCollect(b *testing.B) {
	if os.Getenv("LFEN_POC_SYNTHETIC_ONLY") != "1" {
		b.Skip("explicit synthetic-only opt-in required")
	}
	for _, count := range []int{1000, 10000} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			root := b.TempDir()
			body := bytes.Repeat([]byte("x"), 256)
			for i := 0; i < count; i++ {
				writePreparationSource(b, root, fmt.Sprintf("web/group-%03d/file-%05d.ts", i/100, i), body)
			}
			options := preparationOptions()
			options.IncludePaths = []string{"web"}
			var rows []map[string]any
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for round := 0; round < 3; round++ {
					var before, after runtime.MemStats
					runtime.ReadMemStats(&before)
					started := time.Now()
					manifest := requirePreparationManifest(b, root, options)
					elapsed := time.Since(started)
					runtime.ReadMemStats(&after)
					if !manifest.Complete || len(manifest.Items) != count {
						b.Fatal("incomplete scale observation")
					}
					rows = append(rows, map[string]any{"round": round + 1, "files": count, "source_bytes": count * len(body), "elapsed_ms": float64(elapsed.Microseconds()) / 1000, "allocated_bytes": after.TotalAlloc - before.TotalAlloc, "heap_alloc_before": before.HeapAlloc, "heap_alloc_after": after.HeapAlloc, "process_memory": preparationProcessMemory(), "manifest_sha256": manifest.SHA256})
				}
			}
			b.StopTimer()
			if output := os.Getenv("LFEN_SOURCE_PREPARATION_EVIDENCE"); output != "" {
				if err := os.MkdirAll(output, 0700); err != nil {
					b.Fatal(err)
				}
				encoded, _ := json.MarshalIndent(map[string]any{"runtime": runtime.Version(), "goos": runtime.GOOS, "goarch": runtime.GOARCH, "note": "Synthetic files; 3 observations per benchmark iteration; process memory includes package/test runtime, HWM is cumulative; not p95 or a million-file test.", "rows": rows}, "", "  ")
				if err := os.WriteFile(filepath.Join(output, fmt.Sprintf("collect-%d.json", count)), encoded, 0600); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func preparationProcessMemory() map[string]string {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return map[string]string{"availability": "unavailable on this platform"}
	}
	result := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		for _, key := range []string{"VmRSS:", "VmHWM:"} {
			if strings.HasPrefix(line, key) {
				result[strings.TrimSuffix(key, ":")] = strings.TrimSpace(strings.TrimPrefix(line, key))
			}
		}
	}
	return result
}
