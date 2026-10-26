package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"

	"ragflow/internal/entity"

	"golang.org/x/net/html"
)

const knowledgeWebMaxBytes = 2 << 20

// KnowledgeWebParseError carries only a frozen machine code, never source text.
type KnowledgeWebParseError struct{ Code string }

func (e *KnowledgeWebParseError) Error() string { return e.Code }
func webParseError(code string) error           { return &KnowledgeWebParseError{Code: code} }

// KnowledgeWebEntryContent is an unpersisted, bounded observation product.
// The dataset service owns authorization, live heads and immutable publication.
type KnowledgeWebEntryContent struct {
	Entry      entity.KnowledgeWebEntry
	Decoded    []byte
	Reading    []byte
	References []entity.KnowledgeWebReference
	Warnings   []string
	ParseNodes int
}

// KnowledgeWebParseResult contains no fabricated source or revision identity.
type KnowledgeWebParseResult struct {
	MediaType       string
	InputEntryCount int
	DuplicateCount  int
	Entries         []KnowledgeWebEntryContent
	Warnings        []string
	DecodedBytes    int64
	ReadingBytes    int64
	MetadataBytes   int64
	ReferenceBytes  int64
	ReferenceCount  int64
	ParseNodes      int
}

// KnowledgeWebProducerVersions pins all interpretation used by C2 0.6.0.
func KnowledgeWebProducerVersions() entity.KnowledgeWebProducerVersions {
	return entity.KnowledgeWebProducerVersions{NormalizerVersion: "web-normalizer/0.6.0", ConverterVersion: "web-reading/0.6.0", SanitizerVersion: "web-sanitizer/0.6.0", ResourcePolicyVersion: "web-resources/0.6.0"}
}

// NormalizeKnowledgeWebURL applies the frozen locator rules without contacting
// DNS. Network eligibility is a separate check at every outgoing request.
func NormalizeKnowledgeWebURL(base, raw string, allowFragment bool) (string, error) {
	if len(raw) > 2048 || !utf8.ValidString(raw) || strings.ContainsAny(raw, "\x00\r\n\t") {
		return "", webParseError("knowledge_invalid_request")
	}
	for _, r := range raw {
		if r < 0x20 || r == 0x7f {
			return "", webParseError("knowledge_invalid_request")
		}
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", webParseError("knowledge_invalid_request")
	}
	if base != "" {
		b, err := url.Parse(base)
		if err != nil {
			return "", webParseError("knowledge_invalid_request")
		}
		u = b.ResolveReference(u)
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Opaque != "" || u.Hostname() == "" || (!allowFragment && u.Fragment != "") {
		return "", webParseError("knowledge_invalid_request")
	}
	for _, r := range u.Hostname() {
		if r > 127 || r <= 32 {
			return "", webParseError("knowledge_invalid_request")
		}
	}
	if p := u.Port(); p != "" && !(u.Scheme == "http" && p == "80") && !(u.Scheme == "https" && p == "443") {
		return "", webParseError("knowledge_invalid_request")
	}
	u.Host = strings.ToLower(u.Host)
	if p := u.Port(); p != "" {
		u.Host = strings.TrimSuffix(u.Host, ":"+p)
	}
	if u.Path == "" {
		u.Path = "/"
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", webParseError("knowledge_invalid_request")
	}
	for key := range query {
		switch strings.ToLower(key) {
		case "key", "token", "api_key", "access_token", "password", "secret", "signature":
			return "", webParseError("knowledge_invalid_request")
		}
	}
	value := u.String()
	if len(value) > 2048 {
		return "", webParseError("knowledge_invalid_request")
	}
	return value, nil
}

func knowledgeWebPointer(value string) *string { return &value }
func knowledgeWebNullable(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
func knowledgeWebJSON(value any) []byte { raw, _ := json.Marshal(value); return raw }
func webStringBound(value string, max int) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) <= max && strings.IndexByte(value, 0) < 0
}

func knowledgeWebReference(relation, raw, base, label, media string) (entity.KnowledgeWebReference, error) {
	ref := entity.KnowledgeWebReference{Relation: relation, RawHref: raw, BaseURL: base, Label: label, DeclaredMediaType: knowledgeWebNullable(media)}
	if !webStringBound(raw, 2048) || !webStringBound(label, 256) || !webStringBound(media, 128) {
		return ref, webParseError("web_budget_exceeded")
	}
	if resolved, err := NormalizeKnowledgeWebURL(base, raw, true); err == nil {
		ref.ResolvedURL = &resolved
		ref.OpenAllowed = true
		if u, err := url.Parse(resolved); err == nil && u.Fragment != "" {
			ref.Fragment = knowledgeWebPointer(u.Fragment)
		}
	}
	if ref.Fragment != nil && !webStringBound(*ref.Fragment, 1024) {
		return ref, webParseError("web_budget_exceeded")
	}
	id, err := entity.KnowledgeWebDigest("reference", ref)
	if err != nil {
		return ref, err
	}
	ref.ReferenceID = id
	if len(knowledgeWebJSON(ref)) > 16384 {
		return ref, webParseError("web_budget_exceeded")
	}
	return ref, nil
}

func finishKnowledgeWebEntry(ctx context.Context, basis, value string, position int, body []byte, media string, metadata entity.KnowledgeWebMetadata, extra []entity.KnowledgeWebReference, classification entity.KnowledgeDirectoryClassification) (*KnowledgeWebEntryContent, error) {
	slices.Sort(metadata.Categories)
	metadata.Categories = slices.Compact(metadata.Categories)
	if len(body) > knowledgeWebMaxBytes || !webStringBound(value, 2048) || !webStringBound(metadata.Title, 128) || len(metadata.Authors) > 8 || len(metadata.Categories) > 16 {
		return nil, webParseError("web_budget_exceeded")
	}
	for _, a := range metadata.Authors {
		if !webStringBound(a.Name, 256) || (a.Email != nil && !webStringBound(*a.Email, 254)) {
			return nil, webParseError("web_budget_exceeded")
		}
	}
	for _, c := range metadata.Categories {
		if !webStringBound(c, 128) {
			return nil, webParseError("web_budget_exceeded")
		}
	}
	for _, v := range []*string{metadata.Published, metadata.Updated} {
		if v != nil && !webStringBound(*v, 128) {
			return nil, webParseError("web_budget_exceeded")
		}
	}
	for _, v := range []*string{metadata.CanonicalDeclared, metadata.CanonicalResolved} {
		if v != nil && !webStringBound(*v, 2048) {
			return nil, webParseError("web_budget_exceeded")
		}
	}
	limits := knowledgeReadingLimits{InputBytes: knowledgeWebMaxBytes, OutputBytes: knowledgeWebMaxBytes, Nodes: 50000, Depth: 64, References: 256}
	readingBody, readingMedia := body, media
	if len(body) == 0 && metadata.BodyCompleteness == "external_reference" {
		readingBody = []byte("[仅外部引用；未取得正文]")
		readingMedia = "text/plain"
	}
	reading, err := renderKnowledgeReading(ctx, readingBody, metadata.EffectiveBase, readingMedia, limits)
	if err != nil {
		return nil, webParseError("web_budget_exceeded")
	}
	metadata.EffectiveBase = reading.BaseURL
	refs := make([]entity.KnowledgeWebReference, 0, len(reading.References)+len(extra))
	seen := map[string]bool{}
	for _, item := range reading.References {
		r, err := knowledgeWebReference(item.Relation, item.RawHref, item.BaseURL, item.Label, item.MediaType)
		if err != nil {
			return nil, err
		}
		if !seen[r.ReferenceID] {
			refs = append(refs, r)
			seen[r.ReferenceID] = true
		}
	}
	for _, r := range extra {
		if !seen[r.ReferenceID] {
			refs = append(refs, r)
			seen[r.ReferenceID] = true
		}
	}
	if len(refs) > 256 || len(knowledgeWebJSON(refs)) > 524288 || len(knowledgeWebJSON(metadata)) > 8192 {
		return nil, webParseError("web_budget_exceeded")
	}
	key, err := entity.KnowledgeWebDigest("entry_key", map[string]any{"identity_basis": basis, "identity_value": value})
	if err != nil {
		return nil, err
	}
	metadataHash, err := entity.KnowledgeWebDigest("metadata", metadata)
	if err != nil {
		return nil, err
	}
	refHash, err := entity.KnowledgeWebDigest("references", map[string]any{"references": refs})
	if err != nil {
		return nil, err
	}
	versions := KnowledgeWebProducerVersions()
	sanitizer, resource := versions.SanitizerVersion, versions.ResourcePolicyVersion
	representations := []entity.KnowledgeWebRepresentation{
		{Kind: "decoded_content", MediaType: media, Encoding: "utf-8", ByteLength: int64(len(body)), SHA256: knowledgeWebHash(body), ConverterVersion: versions.ConverterVersion},
		{Kind: "safe_reading", MediaType: "text/html", Encoding: "utf-8", ByteLength: int64(len(reading.HTML)), SHA256: reading.SHA256, ConverterVersion: versions.ConverterVersion, SanitizerVersion: &sanitizer, ResourcePolicyVersion: &resource},
	}
	entry := entity.KnowledgeWebEntry{EntryKey: key, IdentityBasis: basis, IdentityValue: value, IdentityStrength: "strong", Title: metadata.Title, BodyCompleteness: metadata.BodyCompleteness, Eligible: true, DecodedByteLength: int64(len(body)), ReadingByteLength: int64(len(reading.HTML)), FirstPosition: int64(position), Positions: []int64{int64(position)}, Metadata: metadata, MetadataSHA256: metadataHash, ReferencesSHA256: refHash, ReferenceCount: int64(len(refs)), Representations: representations, ProducerVersions: versions}
	if basis == "weak_url" {
		entry.IdentityStrength = "weak"
	}
	if basis == "missing" {
		entry.IdentityStrength = "missing"
		entry.Eligible = false
		entry.IneligibleReason = knowledgeWebPointer("identity_missing")
	}
	visible, useful := false, false
	textTokens := html.NewTokenizer(bytes.NewReader(reading.HTML[len(knowledgeReadingPrefix):]))
	for {
		token := textTokens.Next()
		if token == html.ErrorToken {
			break
		}
		if token == html.TextToken && len(bytes.TrimSpace(textTokens.Text())) > 0 {
			visible = true
			break
		}
	}
	for _, ref := range refs {
		if ref.OpenAllowed {
			useful = true
		}
	}
	if !visible && !useful && metadata.BodyCompleteness != "external_reference" {
		entry.Eligible = false
		entry.IneligibleReason = knowledgeWebPointer("body_missing")
	}
	if metadata.BodyCompleteness == "external_reference" && !useful {
		entry.Eligible = false
		entry.IneligibleReason = knowledgeWebPointer("reference_missing")
	}
	input, err := KnowledgeWebSourceInput(entry, classification)
	if err != nil {
		return nil, err
	}
	if entry.Eligible {
		entry.SourceInputSHA256 = &input
	}
	return &KnowledgeWebEntryContent{Entry: entry, Decoded: bytes.Clone(body), Reading: reading.HTML, References: refs, Warnings: reading.Warnings, ParseNodes: reading.ParseNodes}, nil
}

// KnowledgeWebSourceInput fingerprints content and classification, excluding
// capture time, item position and unrelated feed entries.
func KnowledgeWebSourceInput(entry entity.KnowledgeWebEntry, c entity.KnowledgeDirectoryClassification) (string, error) {
	if len(entry.Representations) != 2 {
		return "", webParseError("web_capture_integrity")
	}
	d, r := entry.Representations[0], entry.Representations[1]
	return entity.KnowledgeWebDigest("source_input", map[string]any{"entry_key": entry.EntryKey, "decoded_sha256": d.SHA256, "decoded_byte_length": d.ByteLength, "decoded_media_type": d.MediaType, "reading_sha256": r.SHA256, "reading_byte_length": r.ByteLength, "producer_versions": entry.ProducerVersions, "metadata_sha256": entry.MetadataSHA256, "references_sha256": entry.ReferencesSHA256, "classification": c})
}

// NormalizeKnowledgeWebCapture parses only caller-supplied bytes. It never
// acquires pages, attachments or models and never creates persistent identities.
func NormalizeKnowledgeWebCapture(ctx context.Context, kind, base, declaredMedia string, raw []byte, classification entity.KnowledgeDirectoryClassification) (*KnowledgeWebParseResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(raw) == 0 || len(raw) > knowledgeWebMaxBytes {
		return nil, webParseError("web_too_large")
	}
	if !utf8.Valid(raw) || bytes.IndexByte(raw, 0) >= 0 {
		return nil, webParseError("web_encoding_unsupported")
	}
	base, err := NormalizeKnowledgeWebURL("", base, false)
	if err != nil {
		return nil, err
	}
	result := &KnowledgeWebParseResult{Entries: []KnowledgeWebEntryContent{}, Warnings: []string{}}
	if kind == "web_page" {
		if declaredMedia != "text/html" {
			return nil, webParseError("web_format_unsupported")
		}
		entry, err := normalizeKnowledgeWebPage(ctx, base, raw, classification)
		if err != nil {
			return nil, err
		}
		result.MediaType = "text/html"
		result.InputEntryCount = 1
		result.ParseNodes = entry.ParseNodes
		result.Entries = append(result.Entries, *entry)
	} else if kind == "web_feed" {
		if declaredMedia != "application/rss+xml" && declaredMedia != "application/atom+xml" && declaredMedia != "application/xml" && declaredMedia != "text/xml" {
			return nil, webParseError("web_format_unsupported")
		}
		if err := normalizeKnowledgeWebFeed(ctx, base, declaredMedia, raw, classification, result); err != nil {
			return nil, err
		}
	} else {
		return nil, webParseError("web_format_unsupported")
	}
	for _, product := range result.Entries {
		for _, warning := range product.Warnings {
			if !slices.Contains(result.Warnings, warning) && len(result.Warnings) < 16 {
				result.Warnings = append(result.Warnings, warning)
			}
		}
		result.DecodedBytes += int64(len(product.Decoded))
		result.ReadingBytes += int64(len(product.Reading))
		result.MetadataBytes += int64(len(knowledgeWebJSON(product.Entry.Metadata)))
		result.ReferenceBytes += int64(len(knowledgeWebJSON(product.References)))
		result.ReferenceCount += int64(len(product.References))
	}
	if result.DecodedBytes > 4<<20 || result.ReadingBytes > 8<<20 || result.MetadataBytes > 1<<20 || result.ReferenceBytes > 2<<20 || result.ReferenceCount > 4096 {
		return nil, webParseError("web_budget_exceeded")
	}
	slices.SortFunc(result.Entries, func(a, b KnowledgeWebEntryContent) int { return strings.Compare(a.Entry.EntryKey, b.Entry.EntryKey) })
	return result, nil
}

func normalizeKnowledgeWebPage(ctx context.Context, base string, raw []byte, c entity.KnowledgeDirectoryClassification) (*KnowledgeWebEntryContent, error) {
	limits := knowledgeReadingLimits{InputBytes: knowledgeWebMaxBytes, OutputBytes: knowledgeWebMaxBytes, Nodes: 50000, Depth: 64, References: 256}
	if err := preflightKnowledgeReading(ctx, raw, limits); err != nil {
		return nil, webParseError("web_budget_exceeded")
	}
	doc, err := html.Parse(bytes.NewReader(bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf})))
	if err != nil {
		return nil, webParseError("web_parse_rejected")
	}
	if _, err := checkKnowledgeReadingTree(ctx, doc, limits); err != nil {
		return nil, webParseError("web_budget_exceeded")
	}
	effectiveBase, _ := findKnowledgeReadingBase(doc, base)
	meta := entity.KnowledgeWebMetadata{Authors: []entity.KnowledgeWebAuthor{}, Categories: []string{}, EffectiveBase: base, BodyCompleteness: "unknown"}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Namespace != "" {
			return
		}
		if n.Type == html.ElementNode {
			switch n.Data {
			case "title":
				if meta.Title == "" {
					meta.Title = knowledgeReadingLabel(n)
				}
			case "link":
				rel, _ := knowledgeReadingAttr(n, "rel")
				if meta.CanonicalDeclared == nil && strings.EqualFold(rel, "canonical") {
					if href, ok := knowledgeReadingAttr(n, "href"); ok {
						meta.CanonicalDeclared = &href
						if resolved, err := NormalizeKnowledgeWebURL(effectiveBase, href, true); err == nil {
							meta.CanonicalResolved = &resolved
						}
					}
				}
			case "meta":
				name, _ := knowledgeReadingAttr(n, "name")
				prop, _ := knowledgeReadingAttr(n, "property")
				content, _ := knowledgeReadingAttr(n, "content")
				switch strings.ToLower(name + prop) {
				case "author":
					meta.Authors = append(meta.Authors, entity.KnowledgeWebAuthor{Name: content})
				case "article:published_time":
					meta.Published = knowledgeWebPointer(content)
				case "article:modified_time":
					meta.Updated = knowledgeWebPointer(content)
				case "article:tag":
					meta.Categories = append(meta.Categories, content)
				}
			}
		}
		if n.Data != "head" && knowledgeReadingDropped[n.Data] {
			return
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	return finishKnowledgeWebEntry(ctx, "page", "page", 0, raw, "text/html", meta, nil, c)
}
