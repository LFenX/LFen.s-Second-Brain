package connector

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
)

// Rendering is package-private and performs no HTTP, persistence, ingestion or
// model execution. The web capture normalizer owns its typed metadata products.
type knowledgeReadingLimits struct {
	InputBytes  int
	OutputBytes int
	Nodes       int
	Depth       int
	References  int
}

type knowledgeReadingReference struct {
	Relation    string
	RawHref     string
	BaseURL     string
	ResolvedURL string
	Fragment    string
	Label       string
	MediaType   string
}

type knowledgeReading struct {
	HTML         []byte
	SHA256       string
	BaseURL      string
	References   []knowledgeReadingReference
	Warnings     []string
	ParseNodes   int
	referenceIDs map[knowledgeReadingReference]int
}

const knowledgeReadingCSP = "default-src 'none'; style-src 'unsafe-inline'; connect-src 'none'; img-src 'none'; media-src 'none'; object-src 'none'; frame-src 'none'; base-uri 'none'; form-action 'none'"

const knowledgeReadingPrefix = `<!doctype html><html><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="` + knowledgeReadingCSP + `"><style>body{margin:16px;font:14px/1.65 sans-serif;overflow-wrap:anywhere}pre{white-space:pre;overflow:auto;padding:12px;background:#f5f5f5}code{font-family:monospace}table{border-collapse:collapse;max-width:100%}td,th{border:1px solid #ccc;padding:6px;vertical-align:top}span[data-knowledge-ref]{text-decoration:underline dotted}</style></head><body>`

var knowledgeReadingAllowed = map[string]bool{
	"p": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"ul": true, "ol": true, "li": true, "blockquote": true, "pre": true, "code": true,
	"table": true, "thead": true, "tbody": true, "tfoot": true, "tr": true, "th": true,
	"td": true, "br": true, "hr": true, "strong": true, "em": true, "del": true,
	"sub": true, "sup": true, "span": true,
}

var knowledgeReadingDropped = map[string]bool{
	"head": true, "script": true, "style": true, "noscript": true, "iframe": true,
	"object": true, "embed": true, "svg": true, "math": true, "template": true,
	"form": true, "input": true, "button": true, "textarea": true, "select": true,
	"audio": true, "video": true, "source": true, "track": true,
	"meta": true, "base": true, "link": true,
}

var knowledgeReadingVoid = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true, "hr": true,
	"img": true, "input": true, "link": true, "meta": true, "param": true,
	"source": true, "track": true, "wbr": true,
}

func renderKnowledgeReading(ctx context.Context, original []byte, baseURL, mediaType string, limits knowledgeReadingLimits) (*knowledgeReading, error) {
	if limits.InputBytes < 1 || limits.OutputBytes < 1 || limits.Nodes < 1 || limits.Depth < 1 || limits.References < 0 ||
		len(original) > limits.InputBytes || !utf8.Valid(original) || bytes.IndexByte(original, 0) >= 0 {
		return nil, fmt.Errorf("invalid offline reading input or limits")
	}
	base, err := resolveKnowledgeReadingURL("", baseURL)
	if err != nil {
		return nil, err
	}
	result := &knowledgeReading{BaseURL: base, References: []knowledgeReadingReference{}, referenceIDs: map[knowledgeReadingReference]int{}}
	writer := &knowledgeReadingWriter{ctx: ctx, limit: limits.OutputBytes}
	writer.write(knowledgeReadingPrefix)
	if mediaType == "text/plain" {
		result.ParseNodes = 1
		writer.write("<pre><code>")
		writer.text(string(original))
		writer.write("</code></pre>")
	} else if mediaType == "text/html" || mediaType == "application/xhtml+xml" {
		if err := preflightKnowledgeReading(ctx, original, limits); err != nil {
			return nil, err
		}
		doc, err := html.Parse(bytes.NewReader(original))
		if err != nil {
			return nil, err
		}
		count, err := checkKnowledgeReadingTree(ctx, doc, limits)
		if err != nil {
			return nil, err
		}
		result.ParseNodes = count
		result.BaseURL, result.Warnings = findKnowledgeReadingBase(doc, base)
		if err := writeKnowledgeReadingNode(writer, doc, result, limits, result.BaseURL, mediaType == "application/xhtml+xml"); err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("unsupported offline reading media type")
	}
	writer.write("</body></html>")
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if writer.err != nil {
		return nil, writer.err
	}
	result.HTML = bytes.Clone(writer.buffer.Bytes())
	result.SHA256 = knowledgeWebHash(result.HTML)
	result.referenceIDs = nil
	return result, nil
}

func preflightKnowledgeReading(ctx context.Context, original []byte, limits knowledgeReadingLimits) error {
	tokenizer := html.NewTokenizer(bytes.NewReader(original))
	tokenizer.SetMaxBuf(limits.InputBytes)
	stack := make([]string, 0, min(limits.Depth, 64))
	for count := 0; ; count++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if count > limits.Nodes*4 {
			return fmt.Errorf("offline reading token budget exceeded")
		}
		kind := tokenizer.Next()
		if kind == html.ErrorToken {
			if tokenizer.Err() == io.EOF {
				return nil
			}
			return tokenizer.Err()
		}
		if kind != html.StartTagToken && kind != html.SelfClosingTagToken && kind != html.EndTagToken {
			continue
		}
		name, _ := tokenizer.TagName()
		tag := string(name)
		if kind == html.EndTagToken {
			for i := len(stack) - 1; i >= 0; i-- {
				if stack[i] == tag {
					stack = stack[:i]
					break
				}
			}
		} else if !knowledgeReadingVoid[tag] {
			stack = append(stack, tag)
			if len(stack) > limits.Depth {
				return fmt.Errorf("offline reading lexical depth exceeded")
			}
		}
	}
}

func checkKnowledgeReadingTree(ctx context.Context, root *html.Node, limits knowledgeReadingLimits) (int, error) {
	type entry struct {
		node  *html.Node
		depth int
	}
	stack := []entry{{node: root}}
	count := 0
	for len(stack) > 0 {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		count++
		if count > limits.Nodes || current.depth > limits.Depth {
			return 0, fmt.Errorf("offline reading tree budget exceeded")
		}
		for child := current.node.LastChild; child != nil; child = child.PrevSibling {
			stack = append(stack, entry{node: child, depth: current.depth + 1})
		}
	}
	return count, nil
}

func findKnowledgeReadingBase(root *html.Node, fallback string) (string, []string) {
	stack := []*html.Node{root}
	result, found, count := fallback, false, 0
	warnings := []string{}
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if current.Type == html.ElementNode && current.Namespace == "" && current.Data == "base" {
			if href, ok := knowledgeReadingAttr(current, "href"); ok {
				count++
				if resolved, err := resolveKnowledgeReadingURL(fallback, href); err == nil {
					if !found {
						result, found = resolved, true
					}
				} else if !slices.Contains(warnings, "invalid_base_ignored") {
					warnings = append(warnings, "invalid_base_ignored")
				}
			}
		}
		if current.Namespace != "" || (current.Data != "head" && knowledgeReadingDropped[current.Data]) {
			continue
		}
		for child := current.LastChild; child != nil; child = child.PrevSibling {
			stack = append(stack, child)
		}
	}
	if count > 1 {
		warnings = append(warnings, "multiple_html_bases")
	}
	return result, warnings
}

func writeKnowledgeReadingNode(writer *knowledgeReadingWriter, node *html.Node, result *knowledgeReading, limits knowledgeReadingLimits, baseURL string, xmlBase bool) error {
	if err := writer.ctx.Err(); err != nil {
		return err
	}
	if writer.err != nil {
		return writer.err
	}
	if node.Type == html.TextNode {
		writer.text(node.Data)
		return writer.err
	}
	if node.Type != html.DocumentNode && node.Type != html.ElementNode {
		return nil
	}
	if node.Namespace != "" || knowledgeReadingDropped[node.Data] {
		return nil
	}
	if xmlBase {
		if declared, ok := knowledgeReadingAttr(node, "xml:base"); ok {
			if resolved, err := resolveKnowledgeReadingURL(baseURL, declared); err == nil {
				baseURL = resolved
			} else if !slices.Contains(result.Warnings, "invalid_base_ignored") {
				result.Warnings = append(result.Warnings, "invalid_base_ignored")
			}
		}
	}
	if node.Data == "a" || node.Data == "img" {
		attribute, relation := "href", "link"
		if node.Data == "img" {
			attribute, relation = "src", "image"
		}
		if raw, ok := knowledgeReadingAttr(node, attribute); ok {
			resolved, _ := resolveKnowledgeReadingURL(baseURL, raw)
			fragment := ""
			if parsed, err := url.Parse(resolved); err == nil {
				fragment = parsed.Fragment
			}
			label := ""
			if node.Data == "img" {
				label, _ = knowledgeReadingAttr(node, "alt")
			} else {
				label = knowledgeReadingLabel(node)
			}
			media, _ := knowledgeReadingAttr(node, "type")
			reference := knowledgeReadingReference{Relation: relation, RawHref: raw, BaseURL: baseURL, ResolvedURL: resolved, Fragment: fragment, Label: label, MediaType: media}
			index, exists := result.referenceIDs[reference]
			if !exists {
				if len(result.References) >= limits.References {
					return fmt.Errorf("offline reading reference budget exceeded")
				}
				result.References = append(result.References, reference)
				index = len(result.References)
				result.referenceIDs[reference] = index
			}
			writer.write(`<span data-knowledge-ref="` + strconv.Itoa(index) + `">`)
			if node.Data == "img" {
				if label == "" {
					label = "[图片引用]"
				}
				writer.text(label)
			}
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				if err := writeKnowledgeReadingNode(writer, child, result, limits, baseURL, xmlBase); err != nil {
					return err
				}
			}
			writer.write("</span>")
			return writer.err
		}
	}
	tag := node.Data
	if tag == "b" {
		tag = "strong"
	} else if tag == "i" {
		tag = "em"
	} else if tag == "s" {
		tag = "del"
	}
	allowed := knowledgeReadingAllowed[tag]
	if allowed {
		writer.write("<" + tag)
		if tag == "code" {
			if classes, ok := knowledgeReadingAttr(node, "class"); ok {
				for _, class := range strings.Fields(classes) {
					if isKnowledgeReadingLanguage(class) {
						writer.write(` class="` + class + `"`)
						break
					}
				}
			}
		}
		if tag == "td" || tag == "th" {
			for _, attribute := range []string{"rowspan", "colspan"} {
				if value, ok := knowledgeReadingAttr(node, attribute); ok {
					if number, err := strconv.Atoi(value); err == nil && number >= 1 && number <= 100 {
						writer.write(" " + attribute + `="` + strconv.Itoa(number) + `"`)
					}
				}
			}
		}
		writer.write(">")
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if err := writeKnowledgeReadingNode(writer, child, result, limits, baseURL, xmlBase); err != nil {
			return err
		}
	}
	if allowed && tag != "br" && tag != "hr" {
		writer.write("</" + tag + ">")
	}
	return writer.err
}

func knowledgeReadingAttr(node *html.Node, key string) (string, bool) {
	for _, attribute := range node.Attr {
		if attribute.Namespace == "" && attribute.Key == key {
			return attribute.Val, true
		}
	}
	return "", false
}

func isKnowledgeReadingLanguage(value string) bool {
	if !strings.HasPrefix(value, "language-") || len(value) < 10 || len(value) > 41 {
		return false
	}
	for _, character := range value[9:] {
		if !(character >= 'a' && character <= 'z') && !(character >= 'A' && character <= 'Z') && !(character >= '0' && character <= '9') && character != '_' && character != '-' {
			return false
		}
	}
	return true
}

func knowledgeReadingLabel(root *html.Node) string {
	var result strings.Builder
	stack := []*html.Node{root}
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if current.Namespace != "" || knowledgeReadingDropped[current.Data] {
			continue
		}
		if current.Type == html.TextNode {
			result.WriteString(current.Data)
		}
		for child := current.LastChild; child != nil; child = child.PrevSibling {
			stack = append(stack, child)
		}
	}
	return result.String()
}

func resolveKnowledgeReadingURL(baseURL, value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return "", err
	}
	if baseURL != "" {
		parent, err := url.Parse(baseURL)
		if err != nil {
			return "", err
		}
		parsed = parent.ResolveReference(parsed)
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil || parsed.Opaque != "" {
		return "", fmt.Errorf("unsupported offline reading reference")
	}
	parsed.Host = strings.ToLower(parsed.Host)
	if (parsed.Scheme == "https" && parsed.Port() == "443") || (parsed.Scheme == "http" && parsed.Port() == "80") {
		parsed.Host = strings.TrimSuffix(parsed.Host, ":"+parsed.Port())
	}
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	return parsed.String(), nil
}

type knowledgeReadingWriter struct {
	ctx    context.Context
	buffer bytes.Buffer
	limit  int
	err    error
}

func (w *knowledgeReadingWriter) write(value string) {
	if w.err != nil {
		return
	}
	if err := w.ctx.Err(); err != nil {
		w.err = err
		return
	}
	if len(value) > w.limit-w.buffer.Len() {
		w.err = fmt.Errorf("offline reading output budget exceeded")
		return
	}
	w.buffer.WriteString(value)
}

func (w *knowledgeReadingWriter) text(value string) {
	for start, current := range value {
		if start%4096 == 0 {
			if err := w.ctx.Err(); err != nil {
				w.err = err
				return
			}
		}
		switch current {
		case '&':
			w.write("&amp;")
		case '<':
			w.write("&lt;")
		case '>':
			w.write("&gt;")
		case '"':
			w.write("&#34;")
		case '\'':
			w.write("&#39;")
		default:
			w.write(string(current))
		}
		if w.err != nil {
			return
		}
	}
}
