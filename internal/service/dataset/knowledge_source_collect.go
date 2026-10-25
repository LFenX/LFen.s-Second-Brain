package dataset

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"ragflow/internal/entity"
	"ragflow/internal/utility"
)

// These private preparation records are not C2 DTOs or persisted domain models.
// No runtime route or syncer invokes the collector.
type knowledgeSourceRule struct {
	Prefix        string
	ComponentSide string
	Language      string
}

type knowledgeCollectionOptions struct {
	RegistrationKey string
	IncludePaths    []string
	Rules           []knowledgeSourceRule
	MaxFiles        int
	MaxFileBytes    int64
}

type knowledgeSourceItem struct {
	Path          string
	Kind          string
	MediaType     string
	ByteLength    int64
	SHA256        string
	ComponentSide string
	Language      string
	State         string
	C2Eligible    bool
}

type knowledgeSourceExclusion struct {
	Path   string
	Reason string
}

type knowledgeSourceManifest struct {
	RegistrationKey string
	ScopeSHA256     string
	Items           []knowledgeSourceItem
	Excluded        []knowledgeSourceExclusion
	Complete        bool
	SHA256          string
}

func prepareKnowledgeCollection(options knowledgeCollectionOptions) (knowledgeCollectionOptions, error) {
	if !knowledgeImportKey.MatchString(options.RegistrationKey) || len(options.RegistrationKey) > 64 || options.MaxFiles < 1 || options.MaxFileBytes < 1 || options.MaxFileBytes > 1<<30 || len(options.IncludePaths) == 0 {
		return options, fmt.Errorf("invalid preparation scope or limits")
	}
	options.IncludePaths = slices.Clone(options.IncludePaths)
	slices.Sort(options.IncludePaths)
	for i, value := range options.IncludePaths {
		if value != "." && !isKnowledgeImportPath(value) {
			return options, fmt.Errorf("invalid or excluded selection: %s", value)
		}
		for _, prior := range options.IncludePaths[:i] {
			if knowledgePathWithin(value, prior) {
				return options, fmt.Errorf("overlapping selections")
			}
		}
	}
	options.Rules = slices.Clone(options.Rules)
	slices.SortFunc(options.Rules, func(a, b knowledgeSourceRule) int { return strings.Compare(a.Prefix, b.Prefix) })
	for i, rule := range options.Rules {
		if (rule.Prefix != "." && !isKnowledgeImportPath(rule.Prefix)) || !validKnowledgeSide(rule.ComponentSide) || (rule.Language != "" && !validKnowledgeText(rule.Language, 32)) || rule.Language != strings.TrimSpace(rule.Language) || (i > 0 && rule.Prefix == options.Rules[i-1].Prefix) {
			return options, fmt.Errorf("invalid classification rule")
		}
	}
	return options, nil
}

func knowledgePathWithin(value, prefix string) bool {
	return prefix == "." || value == prefix || strings.HasPrefix(value, prefix+"/")
}

func knowledgeCollectionClass(name string, rules []knowledgeSourceRule) (string, string) {
	side, language, length := "non_code", "", -1
	for _, rule := range rules {
		if knowledgePathWithin(name, rule.Prefix) && len(rule.Prefix) > length {
			side, language, length = rule.ComponentSide, rule.Language, len(rule.Prefix)
		}
	}
	return side, language
}

func collectKnowledgeSources(ctx context.Context, rootPath string, options knowledgeCollectionOptions) (*knowledgeSourceManifest, error) {
	options, err := prepareKnowledgeCollection(options)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(rootPath)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("selection root must be an existing non-link directory")
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	manifest := &knowledgeSourceManifest{RegistrationKey: options.RegistrationKey, Complete: true, Items: []knowledgeSourceItem{}, Excluded: []knowledgeSourceExclusion{}}
	manifest.ScopeSHA256 = knowledgePreparationHash(options)
	seen := make(map[string]bool)
	buffer := make([]byte, 32*1024)
	visited := 0
	err = fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		visited++
		if visited > options.MaxFiles*4+1024 {
			return fmt.Errorf("directory entry limit exceeded")
		}
		selected, ancestor := false, false
		for _, prefix := range options.IncludePaths {
			selected = selected || knowledgePathWithin(name, prefix)
			ancestor = ancestor || knowledgePathWithin(prefix, name)
			if name == prefix {
				seen[prefix] = true
			}
		}
		if !selected && !ancestor {
			manifest.Excluded = append(manifest.Excluded, knowledgeSourceExclusion{Path: name, Reason: "outside_selection"})
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if walkErr != nil {
			manifest.Complete = false
			manifest.Excluded = append(manifest.Excluded, knowledgeSourceExclusion{Path: name, Reason: "unreadable"})
			return nil
		}
		if name == "." {
			return nil
		}
		if !isKnowledgeImportPath(name) {
			manifest.Excluded = append(manifest.Excluded, knowledgeSourceExclusion{Path: name, Reason: "excluded_path"})
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			manifest.Complete = false
			manifest.Excluded = append(manifest.Excluded, knowledgeSourceExclusion{Path: name, Reason: "link"})
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if len(manifest.Items) >= options.MaxFiles {
			return fmt.Errorf("file count limit exceeded")
		}
		item := observeKnowledgeSource(ctx, root, name, options.MaxFileBytes, buffer)
		item.ComponentSide, item.Language = knowledgeCollectionClass(name, options.Rules)
		if item.State != "observed" {
			manifest.Complete = false
		}
		manifest.Items = append(manifest.Items, item)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for _, selected := range options.IncludePaths {
		if !seen[selected] {
			manifest.Complete = false
			manifest.Excluded = append(manifest.Excluded, knowledgeSourceExclusion{Path: selected, Reason: "missing_selection"})
		}
	}
	slices.SortFunc(manifest.Items, func(a, b knowledgeSourceItem) int { return strings.Compare(a.Path, b.Path) })
	slices.SortFunc(manifest.Excluded, func(a, b knowledgeSourceExclusion) int {
		return strings.Compare(a.Path+"\x00"+a.Reason, b.Path+"\x00"+b.Reason)
	})
	manifest.SHA256 = knowledgePreparationHash(manifest)
	return manifest, nil
}

func observeKnowledgeSource(ctx context.Context, root *os.Root, name string, limit int64, buffer []byte) knowledgeSourceItem {
	item := knowledgeSourceItem{Path: name, State: "unreadable", ByteLength: -1}
	item.Kind, item.MediaType = knowledgeSourceMedia(name)
	before, err := root.Lstat(filepath.FromSlash(name))
	if err != nil || !before.Mode().IsRegular() {
		item.State = "not_regular"
		return item
	}
	item.ByteLength = before.Size()
	if before.Size() > limit {
		item.State = "too_large"
		return item
	}
	file, err := root.Open(filepath.FromSlash(name))
	if err != nil {
		return item
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		item.State = "changed_during_read"
		return item
	}
	hasher := sha256.New()
	probe := &knowledgeSourceProbe{valid: true}
	reader := &knowledgeContextReader{ctx: ctx, reader: file}
	n, err := io.CopyBuffer(io.MultiWriter(hasher, probe), io.LimitReader(reader, limit+1), buffer)
	if err != nil {
		return item
	}
	after, statErr := root.Lstat(filepath.FromSlash(name))
	closed, fileErr := file.Stat()
	if statErr != nil || fileErr != nil || !os.SameFile(before, after) || !os.SameFile(before, closed) || before.Size() != n || before.Size() != after.Size() || before.ModTime() != after.ModTime() || opened.ModTime() != closed.ModTime() || after.Mode()&os.ModeSymlink != 0 {
		item.State = "changed_during_read"
		return item
	}
	if n > limit {
		item.State = "too_large"
		return item
	}
	if item.Kind == "text" && (!probe.valid || len(probe.pending) != 0 || probe.nul) {
		item.State = "invalid_text"
		return item
	}
	if probe.secret {
		item.State = "secret_marker"
		return item
	}
	item.State = "observed"
	item.SHA256 = hex.EncodeToString(hasher.Sum(nil))
	item.C2Eligible = item.Kind == "text" && n <= entity.KnowledgeMaxSourceBytes
	return item
}

type knowledgeContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *knowledgeContextReader) Read(data []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(data)
}

type knowledgeSourceProbe struct {
	valid      bool
	nul        bool
	secret     bool
	pending    []byte
	secretTail []byte
}

func (p *knowledgeSourceProbe) Write(data []byte) (int, error) {
	n := len(data)
	p.nul = p.nul || bytes.IndexByte(data, 0) >= 0
	for _, marker := range []string{"-----BEGIN PRIVATE KEY-----", "-----BEGIN RSA PRIVATE KEY-----"} {
		prefix := data[:min(len(data), len(marker)-1)]
		boundary := append(slices.Clone(p.secretTail), prefix...)
		p.secret = p.secret || bytes.Contains(data, []byte(marker)) || bytes.Contains(boundary, []byte(marker))
	}
	if len(data) >= 32 {
		p.secretTail = append(p.secretTail[:0], data[len(data)-32:]...)
	} else {
		p.secretTail = append(p.secretTail, data...)
		if len(p.secretTail) > 32 {
			p.secretTail = p.secretTail[len(p.secretTail)-32:]
		}
	}
	if !p.valid {
		return n, nil
	}
	if len(p.pending) > 0 {
		var prefix [utf8.UTFMax]byte
		old := len(p.pending)
		copy(prefix[:], p.pending)
		added := copy(prefix[old:], data)
		if !utf8.FullRune(prefix[:old+added]) {
			p.pending = append(p.pending[:0], prefix[:old+added]...)
			return n, nil
		}
		value, width := utf8.DecodeRune(prefix[:old+added])
		if value == utf8.RuneError && width == 1 {
			p.valid = false
			return n, nil
		}
		data = data[width-old:]
		p.pending = p.pending[:0]
	}
	for trim := 0; trim <= min(3, len(data)); trim++ {
		if utf8.Valid(data[:len(data)-trim]) {
			if trim == 0 {
				return n, nil
			}
			if !utf8.FullRune(data[len(data)-trim:]) {
				p.pending = append(p.pending[:0], data[len(data)-trim:]...)
				return n, nil
			}
		}
	}
	p.valid = false
	return n, nil
}

func knowledgeSourceMedia(name string) (string, string) {
	switch strings.ToLower(path.Ext(name)) {
	case ".pdf":
		return "document", "application/pdf"
	case ".pptx":
		return "document", "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	case ".docx":
		return "document", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".html", ".htm":
		return "text", "text/html; charset=utf-8"
	default:
		if utility.IsKnowledgeTextSource(name, nil) {
			return "text", "text/plain; charset=utf-8"
		}
		return "attachment", "application/octet-stream"
	}
}

func knowledgePreparationHash(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err) // Preparation records contain only fixed, JSON-safe value types.
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}
