package connector

import (
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"mime"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"ragflow/internal/entity"
)

const knowledgeAtomNamespace = "http://www.w3.org/2005/Atom"
const knowledgeXHTMLNamespace = "http://www.w3.org/1999/xhtml"
const knowledgeXMLNamespace = "http://www.w3.org/XML/1998/namespace"

// InspectKnowledgeWebMedia checks the declared family against its first root.
// Complete XML and per-entry validation remain the explicit observation step.
func InspectKnowledgeWebMedia(ctx context.Context, kind, declared string, raw []byte) (string, error) {
	media, params, err := mime.ParseMediaType(declared)
	if err != nil {
		return "", webParseError("web_format_unsupported")
	}
	if charset := params["charset"]; charset != "" && !strings.EqualFold(charset, "utf-8") && !strings.EqualFold(charset, "utf8") {
		return "", webParseError("web_encoding_unsupported")
	}
	raw = bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf})
	if kind == "web_page" && media == "text/html" {
		tokens := html.NewTokenizer(bytes.NewReader(raw))
		tokens.SetMaxBuf(knowledgeWebMaxBytes)
		for {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			switch tokens.Next() {
			case html.ErrorToken:
				return "", webParseError("web_format_unsupported")
			case html.StartTagToken, html.SelfClosingTagToken:
				name, _ := tokens.TagName()
				if string(name) == "rss" || string(name) == "feed" || string(name) == "rdf:rdf" {
					return "", webParseError("web_format_unsupported")
				}
				return "text/html", nil
			}
		}
	}
	if kind != "web_feed" || !slices.Contains([]string{"application/rss+xml", "application/atom+xml", "application/xml", "text/xml"}, media) {
		return "", webParseError("web_format_unsupported")
	}
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		token, err := decoder.Token()
		if err != nil {
			return "", webParseError("web_parse_rejected")
		}
		switch token := token.(type) {
		case xml.Directive:
			return "", webParseError("web_parse_rejected")
		case xml.StartElement:
			if token.Name.Space == knowledgeAtomNamespace && token.Name.Local == "feed" && media != "application/rss+xml" {
				return "application/atom+xml", nil
			}
			if token.Name.Space == "" && token.Name.Local == "rss" && media != "application/atom+xml" {
				for _, attr := range token.Attr {
					if attr.Name.Space == "" && attr.Name.Local == "version" && attr.Value == "2.0" {
						return "application/rss+xml", nil
					}
				}
			}
			return "", webParseError("web_format_unsupported")
		}
	}
}

type webXMLPart struct {
	text  string
	child *webXMLNode
}
type webXMLNode struct {
	name        xml.Name
	attrs       []xml.Attr
	parts       []webXMLPart
	base        string
	invalidBase bool
	parseNodes  int
}

func (n *webXMLNode) attr(space, local string) string {
	for _, a := range n.attrs {
		if a.Name.Space == space && a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}
func (n *webXMLNode) children(space, local string) []*webXMLNode {
	out := []*webXMLNode{}
	for _, p := range n.parts {
		if p.child != nil && p.child.name.Space == space && p.child.name.Local == local {
			out = append(out, p.child)
		}
	}
	return out
}
func (n *webXMLNode) child(space, local string) *webXMLNode {
	items := n.children(space, local)
	if len(items) == 0 {
		return nil
	}
	return items[0]
}
func (n *webXMLNode) text() string {
	var out strings.Builder
	for _, p := range n.parts {
		if p.child != nil {
			out.WriteString(p.child.text())
		} else {
			out.WriteString(p.text)
		}
	}
	return out.String()
}
func webChildText(n *webXMLNode, space, name string) string {
	if c := n.child(space, name); c != nil {
		return c.text()
	}
	return ""
}
func webChildNullable(n *webXMLNode, space, name string) *string {
	if c := n.child(space, name); c != nil {
		return knowledgeWebPointer(c.text())
	}
	return nil
}

func parseKnowledgeWebXML(ctx context.Context, raw []byte, base string) (*webXMLNode, error) {
	decoder := xml.NewDecoder(bytes.NewReader(bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf})))
	decoder.Strict = true
	var root *webXMLNode
	stack := []*webXMLNode{}
	nodes := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, webParseError("web_parse_rejected")
		}
		nodes++
		if nodes > 50000 {
			return nil, webParseError("web_budget_exceeded")
		}
		switch token := token.(type) {
		case xml.Directive:
			return nil, webParseError("web_parse_rejected")
		case xml.StartElement:
			if len(stack) >= 64 {
				return nil, webParseError("web_budget_exceeded")
			}
			n := &webXMLNode{name: token.Name, attrs: append([]xml.Attr(nil), token.Attr...), base: base}
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				n.base = parent.base
				parent.parts = append(parent.parts, webXMLPart{child: n})
			} else {
				if root != nil {
					return nil, webParseError("web_parse_rejected")
				}
				root = n
			}
			if declared := n.attr(knowledgeXMLNamespace, "base"); declared != "" {
				if normalized, err := NormalizeKnowledgeWebURL(n.base, declared, true); err == nil {
					n.base = normalized
				} else {
					n.invalidBase = true
				}
			}
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, webParseError("web_parse_rejected")
			}
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) > 0 {
				n := stack[len(stack)-1]
				n.parts = append(n.parts, webXMLPart{text: string(token)})
			} else if len(bytes.TrimSpace(token)) != 0 {
				return nil, webParseError("web_parse_rejected")
			}
		}
	}
	if root == nil || len(stack) != 0 {
		return nil, webParseError("web_parse_rejected")
	}
	root.parseNodes = nodes
	return root, nil
}

func serializeKnowledgeXHTML(node *webXMLNode, body *strings.Builder) error {
	if node.name.Space != knowledgeXHTMLNamespace {
		return webParseError("web_format_unsupported")
	}
	body.WriteByte('<')
	body.WriteString(node.name.Local)
	for _, a := range node.attrs {
		if a.Name.Space == "xmlns" || a.Name.Local == "xmlns" {
			continue
		}
		name := a.Name.Local
		if a.Name.Space == knowledgeXMLNamespace {
			name = "xml:" + name
		} else if a.Name.Space != "" {
			continue
		}
		body.WriteByte(' ')
		body.WriteString(name)
		body.WriteString(`="`)
		_ = xml.EscapeText(body, []byte(a.Value))
		body.WriteByte('"')
	}
	body.WriteByte('>')
	for _, p := range node.parts {
		if p.child != nil {
			if err := serializeKnowledgeXHTML(p.child, body); err != nil {
				return err
			}
		} else {
			_ = xml.EscapeText(body, []byte(p.text))
		}
	}
	if !knowledgeReadingVoid[node.name.Local] {
		body.WriteString("</")
		body.WriteString(node.name.Local)
		body.WriteByte('>')
	}
	return nil
}

func webAtomBody(n *webXMLNode) ([]byte, string, string, error) {
	if n == nil {
		return nil, "text/plain", "", nil
	}
	typeName := n.attr("", "type")
	if typeName == "" || typeName == "text" || typeName == "html" {
		for _, p := range n.parts {
			if p.child != nil {
				return nil, "", "", webParseError("web_format_unsupported")
			}
		}
		media := "text/plain"
		if typeName == "html" {
			media = "text/html"
		}
		return []byte(n.text()), media, n.base, nil
	}
	if typeName != "xhtml" {
		return nil, "", "", webParseError("web_format_unsupported")
	}
	divs := n.children(knowledgeXHTMLNamespace, "div")
	if len(divs) != 1 {
		return nil, "", "", webParseError("web_format_unsupported")
	}
	for _, p := range n.parts {
		if p.child != nil && p.child != divs[0] || p.child == nil && strings.TrimSpace(p.text) != "" {
			return nil, "", "", webParseError("web_format_unsupported")
		}
	}
	var body strings.Builder
	if err := serializeKnowledgeXHTML(divs[0], &body); err != nil {
		return nil, "", "", err
	}
	return []byte(body.String()), "application/xhtml+xml", n.base, nil
}

func webFeedReference(node *webXMLNode, relation, raw, label, media string) (entity.KnowledgeWebReference, error) {
	return knowledgeWebReference(relation, raw, node.base, label, media)
}

func normalizeKnowledgeWebFeed(ctx context.Context, base, declared string, raw []byte, c entity.KnowledgeDirectoryClassification, result *KnowledgeWebParseResult) error {
	root, err := parseKnowledgeWebXML(ctx, raw, base)
	if err != nil {
		return err
	}
	stack := []*webXMLNode{root}
	result.ParseNodes = root.parseNodes
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n.invalidBase && len(result.Warnings) == 0 {
			result.Warnings = append(result.Warnings, "invalid_base_ignored")
		}
		for _, p := range n.parts {
			if p.child != nil {
				stack = append(stack, p.child)
			}
		}
	}
	var entries []*webXMLNode
	atom := root.name.Space == knowledgeAtomNamespace && root.name.Local == "feed"
	if atom {
		if declared == "application/rss+xml" {
			return webParseError("web_format_unsupported")
		}
		result.MediaType = "application/atom+xml"
		entries = root.children(knowledgeAtomNamespace, "entry")
	} else if root.name.Space == "" && root.name.Local == "rss" && root.attr("", "version") == "2.0" {
		if declared == "application/atom+xml" {
			return webParseError("web_format_unsupported")
		}
		result.MediaType = "application/rss+xml"
		channels := root.children("", "channel")
		if len(channels) != 1 {
			return webParseError("web_parse_rejected")
		}
		entries = channels[0].children("", "item")
	} else {
		return webParseError("web_format_unsupported")
	}
	if len(entries) > 1000 {
		return webParseError("web_budget_exceeded")
	}
	result.InputEntryCount = len(entries)
	seen := map[string]int{}
	for i, n := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		var item *KnowledgeWebEntryContent
		if atom {
			item, err = normalizeKnowledgeAtomEntry(ctx, n, root, i, c)
		} else {
			item, err = normalizeKnowledgeRSSEntry(ctx, n, i, c)
		}
		if err != nil {
			return err
		}
		result.ParseNodes += item.ParseNodes
		if result.ParseNodes > 50000 {
			return webParseError("web_budget_exceeded")
		}
		if previous, ok := seen[item.Entry.EntryKey]; ok {
			prior := &result.Entries[previous]
			if !bytes.Equal(knowledgeWebJSON(prior.Entry.Metadata), knowledgeWebJSON(item.Entry.Metadata)) || prior.Entry.MetadataSHA256 != item.Entry.MetadataSHA256 || prior.Entry.ReferencesSHA256 != item.Entry.ReferencesSHA256 || prior.Entry.Representations[0] != item.Entry.Representations[0] || prior.Entry.Representations[1].SHA256 != item.Entry.Representations[1].SHA256 {
				return webParseError("web_entry_conflict")
			}
			prior.Entry.Positions = append(prior.Entry.Positions, int64(i))
			result.DuplicateCount++
			continue
		}
		seen[item.Entry.EntryKey] = len(result.Entries)
		result.Entries = append(result.Entries, *item)
	}
	return nil
}

func webEntryIdentity(basis, value string, link *webXMLNode, raw string, position int) (string, string) {
	if value != "" {
		return basis, value
	}
	if link != nil && raw != "" {
		if normalized, err := NormalizeKnowledgeWebURL(link.base, raw, true); err == nil {
			u, _ := url.Parse(normalized)
			u.Fragment = ""
			u.RawFragment = ""
			return "weak_url", u.String()
		}
	}
	return "missing", "position:" + strconv.Itoa(position)
}

func normalizeKnowledgeRSSEntry(ctx context.Context, n *webXMLNode, position int, c entity.KnowledgeDirectoryClassification) (*KnowledgeWebEntryContent, error) {
	link := n.child("", "link")
	rawLink := ""
	if link != nil {
		rawLink = link.text()
	}
	basis, value := webEntryIdentity("rss_guid", webChildText(n, "", "guid"), link, rawLink, position)
	meta := entity.KnowledgeWebMetadata{Title: webChildText(n, "", "title"), Authors: []entity.KnowledgeWebAuthor{}, Categories: []string{}, Published: webChildNullable(n, "", "pubDate"), EffectiveBase: n.base, BodyCompleteness: "summary"}
	for _, a := range n.children("", "author") {
		meta.Authors = append(meta.Authors, entity.KnowledgeWebAuthor{Name: a.text()})
	}
	for _, cat := range n.children("", "category") {
		meta.Categories = append(meta.Categories, cat.text())
	}
	body := n.child("http://purl.org/rss/1.0/modules/content/", "encoded")
	if body != nil && body.text() != "" {
		meta.BodyCompleteness = "unknown"
	} else {
		body = n.child("", "description")
	}
	decoded := []byte{}
	if body != nil {
		decoded = []byte(body.text())
		meta.EffectiveBase = body.base
	}
	refs := []entity.KnowledgeWebReference{}
	if link != nil && rawLink != "" {
		ref, err := webFeedReference(link, "link", rawLink, meta.Title, "")
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	for _, node := range n.children("", "enclosure") {
		ref, err := webFeedReference(node, "enclosure", node.attr("", "url"), "", node.attr("", "type"))
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return finishKnowledgeWebEntry(ctx, basis, value, position, decoded, "text/html", meta, refs, c)
}

func normalizeKnowledgeAtomEntry(ctx context.Context, n, feed *webXMLNode, position int, c entity.KnowledgeDirectoryClassification) (*KnowledgeWebEntryContent, error) {
	links := n.children(knowledgeAtomNamespace, "link")
	var identityLink *webXMLNode
	identityURL := ""
	for _, link := range links {
		rel := link.attr("", "rel")
		if identityLink == nil && (rel == "alternate" || rel == "") {
			identityLink = link
			identityURL = link.attr("", "href")
		}
	}
	basis, value := webEntryIdentity("atom_id", webChildText(n, knowledgeAtomNamespace, "id"), identityLink, identityURL, position)
	meta := entity.KnowledgeWebMetadata{Title: webChildText(n, knowledgeAtomNamespace, "title"), Authors: []entity.KnowledgeWebAuthor{}, Categories: []string{}, Published: webChildNullable(n, knowledgeAtomNamespace, "published"), Updated: webChildNullable(n, knowledgeAtomNamespace, "updated"), EffectiveBase: n.base, BodyCompleteness: "summary"}
	authors := n.children(knowledgeAtomNamespace, "author")
	if len(authors) == 0 {
		authors = feed.children(knowledgeAtomNamespace, "author")
	}
	for _, a := range authors {
		author := entity.KnowledgeWebAuthor{Name: webChildText(a, knowledgeAtomNamespace, "name"), Email: webChildNullable(a, knowledgeAtomNamespace, "email")}
		if uri := a.child(knowledgeAtomNamespace, "uri"); uri != nil {
			if resolved, err := NormalizeKnowledgeWebURL(uri.base, uri.text(), true); err == nil {
				author.URI = &resolved
			}
		}
		meta.Authors = append(meta.Authors, author)
	}
	for _, cat := range n.children(knowledgeAtomNamespace, "category") {
		meta.Categories = append(meta.Categories, cat.attr("", "term"))
	}
	content := n.child(knowledgeAtomNamespace, "content")
	summary := n.child(knowledgeAtomNamespace, "summary")
	chosen := content
	refs := []entity.KnowledgeWebReference{}
	for _, link := range links {
		rel := link.attr("", "rel")
		if rel == "enclosure" || rel == "alternate" || rel == "" {
			relation := "link"
			if rel == "enclosure" {
				relation = "enclosure"
			}
			ref, err := webFeedReference(link, relation, link.attr("", "href"), meta.Title, link.attr("", "type"))
			if err != nil {
				return nil, err
			}
			refs = append(refs, ref)
		}
	}
	if content != nil && content.attr("", "src") != "" {
		meta.BodyCompleteness = "external_reference"
		ref, err := webFeedReference(content, "external_content", content.attr("", "src"), "", content.attr("", "type"))
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
		chosen = summary
	} else if content == nil {
		chosen = summary
	} else {
		meta.BodyCompleteness = "unknown"
	}
	decoded, media, effective, err := webAtomBody(chosen)
	if err != nil {
		return nil, err
	}
	if effective != "" {
		meta.EffectiveBase = effective
	}
	return finishKnowledgeWebEntry(ctx, basis, value, position, decoded, media, meta, refs, c)
}
