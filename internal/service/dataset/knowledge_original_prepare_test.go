// Copyright 2026 LFen's Second Brain contributors.
// SPDX-License-Identifier: Apache-2.0

package dataset

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/zeebo/xxh3"
)

func originalPreparationLimits() knowledgeOriginalLimits {
	return knowledgeOriginalLimits{fileBytes: 64 << 20, officeEntries: 2048, officeExpandedBytes: 256 << 20,
		officeRatio: 100, officeXMLBytes: 2 << 20, officeAllXMLBytes: 8 << 20, officeDirectoryBytes: 2 << 20, officeXMLDepth: 256}
}

func originalPreparationRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "scratch")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	return root
}

func originalPreparationSHA(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}

type originalPreparationPart struct {
	name string
	body string
	mode os.FileMode
}

func originalPreparationOfficeParts(kind string) []originalPreparationPart {
	part, contentType := "word/document.xml", "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"
	main := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>中文合成</w:t></w:r></w:p></w:body></w:document>`
	if kind == "pptx" {
		part, contentType = "ppt/presentation.xml", "application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"
		main = `<p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"><p:sldIdLst/></p:presentation>`
	}
	return []originalPreparationPart{
		{"[Content_Types].xml", `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Override PartName="/` + part + `" ContentType="` + contentType + `"/></Types>`, 0600},
		{"_rels/.rels", `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="` + part + `"/></Relationships>`, 0600},
		{part, main, 0600},
	}
}

func originalPreparationZIP(t *testing.T, parts []originalPreparationPart) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, part := range parts {
		header := &zip.FileHeader{Name: part.name, Method: zip.Deflate}
		header.SetMode(part.mode)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(entry, part.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestKnowledgeOriginalPreparationPreservesBytes(t *testing.T) {
	for _, test := range []struct {
		name, filename string
		body           []byte
	}{
		{"text", "原名.tsx", []byte("\xef\xbb\xbf// 中文 😺\r\nexport const x = 'e\u0301';\r\n")},
		{"pdf", "原名.pdf", []byte("%PDF-1.7\n% binary \xff\x00\n%%EOF\n")},
		{"pdf2", "原名.pdf", []byte("%PDF-2.0\n%%EOF\r\n")},
		{"docx", "原名.docx", originalPreparationZIP(t, originalPreparationOfficeParts("docx"))},
		{"pptx", "原名.pptx", originalPreparationZIP(t, originalPreparationOfficeParts("pptx"))},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := originalPreparationRoot(t)
			media := classifyKnowledgeOriginalMedia(test.filename)
			stage, err := stageKnowledgeOriginal(t.Context(), root, test.filename, "docs/目标"+filepath.Ext(test.filename), media, bytes.NewReader(test.body), int64(len(test.body)), originalPreparationSHA(test.body), originalPreparationLimits())
			if err != nil {
				t.Fatal(err)
			}
			if stage.sha256 != originalPreparationSHA(test.body) || stage.byteLength != int64(len(test.body)) {
				t.Fatal("original metadata changed")
			}
			native := xxh3.Hash128(test.body).Bytes()
			if stage.nativeHash != hex.EncodeToString(native[:]) {
				t.Fatal("native hash is not compatible")
			}
			got, err := io.ReadAll(stage.file)
			if err != nil || !bytes.Equal(got, test.body) {
				t.Fatalf("original bytes changed: %v", err)
			}
			info, err := stage.file.Stat()
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatal("temporary file is not owner-only")
			}
			directory, err := stage.root.Stat(stage.directory)
			if err != nil || directory.Mode().Perm() != 0700 {
				t.Fatal("temporary directory is not owner-only")
			}
			if err := stage.close(); err != nil {
				t.Fatal(err)
			}
			if err := stage.close(); err != nil {
				t.Fatal("cleanup is not idempotent")
			}
			entries, err := os.ReadDir(root)
			if err != nil || len(entries) != 0 {
				t.Fatal("temporary bytes remain after close")
			}
		})
	}
}

func TestKnowledgeOriginalPreparationRejectsAndCleans(t *testing.T) {
	for _, test := range []struct {
		name, filename, path, media string
		body                        []byte
		size                        int64
		hash                        string
		want                        error
	}{
		{"short", "a.txt", "a.txt", "text/plain", []byte("x"), 2, "", errOriginalLength},
		{"long", "a.txt", "a.txt", "text/plain", []byte("xx"), 1, "", errOriginalLength},
		{"digest", "a.txt", "a.txt", "text/plain", []byte("x"), 1, strings.Repeat("0", 64), errOriginalDigest},
		{"utf8", "a.txt", "a.txt", "text/plain", []byte{0xff}, 1, "", errOriginalText},
		{"incomplete utf8", "a.txt", "a.txt", "text/plain", []byte{0xf0, 0x9f}, 2, "", errOriginalText},
		{"nul", "a.txt", "a.txt", "text/plain", []byte{'x', 0}, 2, "", errOriginalText},
		{"secret", "a.txt", "a.txt", "text/plain", []byte("-----BEGIN PRIVATE KEY-----"), 27, "", errOriginalSecret},
		{"type mismatch", "a.pdf", "a.docx", "application/octet-stream", []byte("x"), 1, "", errOriginalType},
		{"PDF spoof", "a.pdf", "a.pdf", "application/pdf", []byte("plain UTF8"), 10, "", errOriginalType},
		{"PDF missing EOF", "a.pdf", "a.pdf", "application/pdf", []byte("%PDF-1.4\ncut"), 12, "", errOriginalType},
		{"credentials", ".env.txt", "safe.txt", "text/plain", []byte("x"), 1, "", errOriginalType},
		{"private directory", "safe.txt", ".ssh/safe.txt", "text/plain", []byte("x"), 1, "", errOriginalType},
		{"filename path", "dir/a.txt", "a.txt", "text/plain", []byte("x"), 1, "", errOriginalType},
		{"filename CRLF", "a\r\n.txt", "a.txt", "text/plain", []byte("x"), 1, "", errOriginalType},
		{"empty", "a.pdf", "a.pdf", "application/pdf", nil, 0, "", errOriginalOptions},
		{"over cap", "a.txt", "a.txt", "text/plain", []byte("x"), 64<<20 + 1, "", errOriginalOptions},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := originalPreparationRoot(t)
			stage, err := stageKnowledgeOriginal(t.Context(), root, test.filename, test.path, test.media, bytes.NewReader(test.body), test.size, test.hash, originalPreparationLimits())
			if !errors.Is(err, test.want) || stage != nil {
				t.Fatalf("expected %v, got %v", test.want, err)
			}
			entries, err := os.ReadDir(root)
			if err != nil || len(entries) != 0 {
				t.Fatal("failure leaked temporary bytes")
			}
		})
	}
}

func TestKnowledgeOriginalPreparationOfficeBounds(t *testing.T) {
	for _, scenario := range []string{"missing part", "wrong type", "wrong relationship", "external relationship", "namespaced type attribute", "DTD", "duplicate XML attribute", "bad XML", "zip slip", "absolute member", "backslash member", "duplicate member", "symlink", "macro", "encrypted", "CRC", "ratio", "entry budget", "forged entry count", "directory budget", "ZIP64", "XML budget", "XML depth"} {
		t.Run(scenario, func(t *testing.T) {
			parts := originalPreparationOfficeParts("docx")
			limits := originalPreparationLimits()
			switch scenario {
			case "missing part":
				parts = parts[:2]
			case "wrong type":
				parts[0].body = strings.ReplaceAll(parts[0].body, "wordprocessingml.document.main+xml", "presentationml.presentation.main+xml")
			case "wrong relationship":
				parts[1].body = strings.ReplaceAll(parts[1].body, "word/document.xml", "word/other.xml")
			case "external relationship":
				parts[1].body = strings.ReplaceAll(parts[1].body, `Target="word/document.xml"`, `Target="word/document.xml" TargetMode="External"`)
			case "namespaced type attribute":
				parts[0].body = strings.ReplaceAll(parts[0].body, `PartName=`, `xmlns:x="urn:fake" x:PartName=`)
			case "DTD":
				parts[2].body = `<!DOCTYPE x [<!ENTITY ext SYSTEM "https://external.invalid/secret">]>` + parts[2].body
			case "duplicate XML attribute":
				parts[2].body = strings.ReplaceAll(parts[2].body, `<w:body>`, `<w:body x="1" x="2">`)
			case "bad XML":
				parts[2].body = `<w:document>`
			case "zip slip":
				parts = append(parts, originalPreparationPart{"../escape", "x", 0600})
			case "absolute member":
				parts = append(parts, originalPreparationPart{"/escape", "x", 0600})
			case "backslash member":
				parts = append(parts, originalPreparationPart{`word\escape`, "x", 0600})
			case "duplicate member":
				parts = append(parts, parts[2])
			case "symlink":
				parts = append(parts, originalPreparationPart{"word/link", "target", os.ModeSymlink | 0777})
			case "macro":
				parts = append(parts, originalPreparationPart{"word/vbaProject.bin", "SYNTHETIC", 0600})
			case "ratio":
				parts = append(parts, originalPreparationPart{"word/large.xml", strings.Repeat("A", 64<<10), 0600})
			case "entry budget":
				limits.officeEntries = 2
			case "directory budget":
				limits.officeDirectoryBytes = 32
			case "XML budget":
				limits.officeXMLBytes = 64
			case "XML depth":
				limits.officeXMLDepth = 2
			}
			body := originalPreparationZIP(t, parts)
			end := bytes.LastIndex(body, []byte{'P', 'K', 5, 6})
			if scenario == "forged entry count" {
				binary.LittleEndian.PutUint16(body[end+8:], 1)
				binary.LittleEndian.PutUint16(body[end+10:], 1)
			}
			if scenario == "ZIP64" {
				binary.LittleEndian.PutUint16(body[end+8:], 65535)
				binary.LittleEndian.PutUint16(body[end+10:], 65535)
			}
			if scenario == "encrypted" {
				central := bytes.Index(body, []byte{'P', 'K', 1, 2})
				binary.LittleEndian.PutUint16(body[central+8:], 1)
			}
			if scenario == "CRC" {
				central := bytes.Index(body, []byte{'P', 'K', 1, 2})
				binary.LittleEndian.PutUint32(body[central+16:], binary.LittleEndian.Uint32(body[central+16:])^1)
			}
			root := originalPreparationRoot(t)
			stage, err := stageKnowledgeOriginal(t.Context(), root, "a.docx", "docs/a.docx", "application/octet-stream", bytes.NewReader(body), int64(len(body)), "", limits)
			if err == nil || stage != nil {
				t.Fatal("unsafe package accepted")
			}
			entries, err := os.ReadDir(root)
			if err != nil || len(entries) != 0 {
				t.Fatal("failed package leaked bytes")
			}
		})
	}
}

func TestKnowledgeOriginalPreparationProbeBoundaries(t *testing.T) {
	inputs := [][]byte{
		nil, []byte("ASCII"), []byte("\xef\xbb\xbf"), []byte("中文"), []byte("😺🧠"), []byte("\uFFFD"),
		[]byte("A中😺文\r\n"), {0x80}, {0xc0, 0xaf}, {0xed, 0xa0, 0x80}, {0xf5, 0x80, 0x80, 0x80},
		{0xf0, 0x9f}, {'x', 0, 'y'}, []byte("前缀-----BEGIN PRIVATE KEY-----尾"), []byte("x-----BEGIN RSA PRIVATE KEY-----"),
	}
	for index, input := range inputs {
		for _, chunk := range []int{1, 2, 3, 4, 7, 64 << 10} {
			probe := &knowledgeSourceProbe{valid: true}
			for start := 0; start < len(input); start += chunk {
				data := input[start:min(len(input), start+chunk)]
				if n, err := probe.Write(data); n != len(data) || err != nil {
					t.Fatalf("input=%d chunk=%d writer contract", index, chunk)
				}
			}
			if (probe.valid && len(probe.pending) == 0) != utf8.Valid(input) || probe.nul != bytes.ContainsRune(input, 0) {
				t.Fatalf("UTF-8/NUL changed: input=%d chunk=%d", index, chunk)
			}
			secret := bytes.Contains(input, []byte("-----BEGIN PRIVATE KEY-----")) || bytes.Contains(input, []byte("-----BEGIN RSA PRIVATE KEY-----"))
			if probe.secret != secret {
				t.Fatalf("secret boundary changed: input=%d chunk=%d", index, chunk)
			}
		}
	}
}

type originalPreparationReader func([]byte) (int, error)

func (f originalPreparationReader) Read(p []byte) (int, error) { return f(p) }

type originalPreparationWriter func([]byte) (int, error)

func (f originalPreparationWriter) Write(p []byte) (int, error) { return f(p) }

func TestKnowledgeOriginalPreparationCancellationAndRoot(t *testing.T) {
	t.Run("write failure", func(t *testing.T) {
		failure := errors.New("synthetic destination failure")
		_, err := copyKnowledgeOriginalBytes(t.Context(), originalPreparationWriter(func([]byte) (int, error) { return 0, failure }), strings.NewReader("data"), 4)
		if !errors.Is(err, failure) {
			t.Fatal("write error was masked")
		}
	})
	t.Run("short write", func(t *testing.T) {
		_, err := copyKnowledgeOriginalBytes(t.Context(), originalPreparationWriter(func([]byte) (int, error) { return 1, nil }), strings.NewReader("data"), 4)
		if !errors.Is(err, io.ErrShortWrite) {
			t.Fatal("short destination write was masked")
		}
	})
	t.Run("cancel mid read", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		root := originalPreparationRoot(t)
		reader := originalPreparationReader(func(p []byte) (int, error) { cancel(); p[0] = 'x'; return 1, nil })
		if stage, err := stageKnowledgeOriginal(ctx, root, "a.txt", "a.txt", "text/plain", reader, 10, "", originalPreparationLimits()); !errors.Is(err, context.Canceled) || stage != nil {
			t.Fatal(err)
		}
		entries, _ := os.ReadDir(root)
		if len(entries) != 0 {
			t.Fatal("cancelled bytes remain")
		}
	})
	t.Run("read failure", func(t *testing.T) {
		root := originalPreparationRoot(t)
		failure := errors.New("synthetic read failure")
		if stage, err := stageKnowledgeOriginal(t.Context(), root, "a.txt", "a.txt", "text/plain", originalPreparationReader(func([]byte) (int, error) { return 0, failure }), 10, "", originalPreparationLimits()); !errors.Is(err, failure) || stage != nil {
			t.Fatal(err)
		}
		entries, _ := os.ReadDir(root)
		if len(entries) != 0 {
			t.Fatal("failed bytes remain")
		}
	})
	t.Run("root path replaced while streaming", func(t *testing.T) {
		root := originalPreparationRoot(t)
		moved := root + "-moved"
		source := strings.NewReader("preserved")
		once := false
		reader := originalPreparationReader(func(p []byte) (int, error) {
			if !once {
				once = true
				if err := os.Rename(root, moved); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(root, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, "keep.txt"), []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			return source.Read(p)
		})
		stage, err := stageKnowledgeOriginal(t.Context(), root, "a.txt", "a.txt", "text/plain", reader, 9, "", originalPreparationLimits())
		if err != nil {
			t.Fatal(err)
		}
		if err := stage.close(); err != nil {
			t.Fatal(err)
		}
		kept, err := os.ReadFile(filepath.Join(root, "keep.txt"))
		if err != nil || string(kept) != "keep" {
			t.Fatal("cleanup followed replaced root")
		}
		entries, _ := os.ReadDir(moved)
		if len(entries) != 0 {
			t.Fatal("pinned root leaked stage")
		}
	})
}

type originalPreparationDenseReader struct {
	remaining int64
	offset    int
}

func (r *originalPreparationDenseReader) Read(p []byte) (int, error) {
	if r.remaining == 0 {
		return 0, io.EOF
	}
	n := min(int64(len(p)), r.remaining)
	pattern := []byte("中文")
	for i := range p[:n] {
		p[i] = pattern[r.offset%len(pattern)]
		r.offset++
	}
	r.remaining -= n
	return int(n), nil
}

func TestKnowledgeOriginalPreparationLargeText(t *testing.T) {
	for _, size := range []int64{3 << 20, (64 << 20) / 6 * 6} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			root := originalPreparationRoot(t)
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			stage, err := stageKnowledgeOriginal(t.Context(), root, "合成.tsx", "frontend/合成.tsx", "text/plain", &originalPreparationDenseReader{remaining: size}, size, "", originalPreparationLimits())
			runtime.ReadMemStats(&after)
			if err != nil {
				t.Fatal(err)
			}
			defer stage.close()
			if stage.byteLength != size {
				t.Fatal("large file truncated")
			}
			allocated := after.TotalAlloc - before.TotalAlloc
			t.Logf("source_bytes=%d allocation_bytes=%d", size, allocated)
			if allocated > 4<<20 {
				t.Fatalf("streaming preparation allocated %d bytes", allocated)
			}
		})
	}
}
