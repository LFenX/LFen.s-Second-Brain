package connector

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"golang.org/x/net/html"
)

func knowledgeReadingTestLimits() knowledgeReadingLimits {
	return knowledgeReadingLimits{InputBytes: 2 << 20, OutputBytes: 2 << 20, Nodes: 50000, Depth: 64, References: 256}
}

func knowledgeReadingTestNodes(t *testing.T, reading *knowledgeReading, tag string) []*html.Node {
	t.Helper()
	doc, err := html.Parse(bytes.NewReader(reading.HTML))
	if err != nil {
		t.Fatal(err)
	}
	var nodes []*html.Node
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == tag {
			nodes = append(nodes, node)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(doc)
	return nodes
}

func knowledgeReadingTestText(node *html.Node) string {
	if node.Type == html.TextNode {
		return node.Data
	}
	var text strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		text.WriteString(knowledgeReadingTestText(child))
	}
	return text.String()
}

func TestKnowledgeReadingPreservesCodeAndTable(t *testing.T) {
	original := []byte("<!doctype html><html><head><base href=\"https://CDN.example.test:443/assets/\"><link rel=\"canonical\" href=\"https://never.example.test/canonical/\"></head><body><h1>合成组件</h1><pre><code class=\"discard language-typescript\">const 名称 = \"🧠\";\r\n\tif (名称) {\r\n    show(名称);\r\n}</code></pre><table><tr><th rowspan=\"2\">组</th><th colspan=\"2\">项</th></tr><tr><td>甲</td><td>乙</td></tr></table><a href=\"../doc.pdf#page=2\" download><em>查看附件</em></a></body></html>")
	copyBefore := bytes.Clone(original)
	reading, err := renderKnowledgeReading(context.Background(), original, "https://page.example.test/article", "text/html", knowledgeReadingTestLimits())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original, copyBefore) || reading.BaseURL != "https://cdn.example.test/assets/" {
		t.Fatal("original mutated or base/canonical confused")
	}
	code := knowledgeReadingTestNodes(t, reading, "code")
	if len(code) != 1 || knowledgeReadingTestText(code[0]) != "const 名称 = \"🧠\";\n\tif (名称) {\n    show(名称);\n}" {
		t.Fatal("HTML-normalized line breaks, tabs, Unicode or code text changed")
	}
	if class, _ := knowledgeReadingAttr(code[0], "class"); class != "language-typescript" {
		t.Fatal("bounded code language was lost")
	}
	headers := knowledgeReadingTestNodes(t, reading, "th")
	if len(headers) != 2 {
		t.Fatal("table headers lost")
	}
	if rowspan, _ := knowledgeReadingAttr(headers[0], "rowspan"); rowspan != "2" {
		t.Fatal("rowspan lost")
	}
	if colspan, _ := knowledgeReadingAttr(headers[1], "colspan"); colspan != "2" {
		t.Fatal("colspan lost")
	}
	if len(reading.References) != 1 || reading.References[0].RawHref != "../doc.pdf#page=2" || reading.References[0].ResolvedURL != "https://cdn.example.test/doc.pdf#page=2" || reading.References[0].Fragment != "page=2" || reading.References[0].Label != "查看附件" {
		t.Fatalf("reference changed: %+v", reading.References)
	}
	sum := sha256.Sum256(reading.HTML)
	if reading.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatal("whole reading hash mismatch")
	}
	repeated, err := renderKnowledgeReading(context.Background(), original, "https://page.example.test/article", "text/html", knowledgeReadingTestLimits())
	if err != nil || !bytes.Equal(repeated.HTML, reading.HTML) || repeated.SHA256 != reading.SHA256 {
		t.Fatal("reading is not deterministic")
	}
}

func TestKnowledgeReadingRemovesActiveContent(t *testing.T) {
	attacks := []string{
		`<script src="https://never.example.test/execute">unsafe-script-text</script><p onclick="run()" style="background:url(https://never.example.test/css)">保留正文</p>`,
		`<meta http-equiv="refresh" content="0;url=https://never.example.test/"><base target="_top"><link rel="stylesheet" href="https://never.example.test/style"><p id="danger" data-anything="1">正文</p>`,
		`<iframe srcdoc="<script>run()</script>" src="https://never.example.test/frame"></iframe><object data="https://never.example.test/object"></object><embed src="https://never.example.test/embed">`,
		`<svg><foreignObject><img src="https://never.example.test/foreign" onerror="run()"></foreignObject></svg><math><mtext><img src="x"></mtext></math>`,
		`<form action="https://never.example.test/submit"><input autofocus onfocus="run()"><button formaction="https://never.example.test/go">send</button></form><video autoplay poster="https://never.example.test/poster"><source src="x"></video>`,
		`<a href="javascript:run()" target="_top">危险引用</a><img src="https://never.example.test/img" srcset="x 2x" onerror="run()" alt="合成图">`,
		`<table><tr><td background="https://never.example.test/bg" rowspan="99999999999999999999" colspan="0">单元格</td></tr></table><code class="language-js:run language-ok" onload="run()">safe</code>`,
		`<template><base href="https://never.example.test/base"><img src="https://never.example.test/template"></template><p>outside</p>`,
		`<noscript><img src="https://never.example.test/noscript"></noscript><style>@import url(https://never.example.test/style)</style><p>safe</p>`,
	}
	for _, attack := range attacks {
		t.Run(attack[:min(35, len(attack))], func(t *testing.T) {
			reading, err := renderKnowledgeReading(context.Background(), []byte(attack), "https://page.example.test/a", "text/html", knowledgeReadingTestLimits())
			if err != nil {
				t.Fatal(err)
			}
			bodies := knowledgeReadingTestNodes(t, reading, "body")
			if len(bodies) != 1 {
				t.Fatal("missing body")
			}
			allowed := map[string]bool{"p": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true, "ul": true, "ol": true, "li": true, "blockquote": true, "pre": true, "code": true, "table": true, "thead": true, "tbody": true, "tfoot": true, "tr": true, "th": true, "td": true, "br": true, "hr": true, "strong": true, "em": true, "del": true, "sub": true, "sup": true, "span": true}
			var check func(*html.Node)
			check = func(node *html.Node) {
				if node.Type == html.ElementNode && node != bodies[0] {
					if !allowed[node.Data] || node.Namespace != "" {
						t.Errorf("active/unexpected node: %s %s", node.Namespace, node.Data)
					}
					for _, attr := range node.Attr {
						if attr.Namespace != "" || !((node.Data == "td" || node.Data == "th") && (attr.Key == "rowspan" || attr.Key == "colspan")) && !(node.Data == "span" && attr.Key == "data-knowledge-ref") && !(node.Data == "code" && attr.Key == "class" && attr.Val == "language-ok") {
							t.Errorf("source attribute survived: %s.%s", node.Data, attr.Key)
						}
					}
				}
				for child := node.FirstChild; child != nil; child = child.NextSibling {
					check(child)
				}
			}
			check(bodies[0])
			if strings.Contains(knowledgeReadingTestText(bodies[0]), "unsafe-script-text") || !strings.Contains(string(reading.HTML), `default-src 'none'`) {
				t.Fatal("executable source text or missing CSP")
			}
			for _, reference := range reading.References {
				if strings.HasPrefix(reference.RawHref, "javascript:") && reference.ResolvedURL != "" {
					t.Fatal("dangerous reference became navigable")
				}
			}
		})
	}
}

func TestKnowledgeReadingPlainTextStaysLiteral(t *testing.T) {
	original := "\n<script>literal()</script>\n  <code>中文 & 🧠</code>"
	reading, err := renderKnowledgeReading(context.Background(), []byte(original), "https://page.example.test/a", "text/plain", knowledgeReadingTestLimits())
	if err != nil {
		t.Fatal(err)
	}
	code := knowledgeReadingTestNodes(t, reading, "code")
	if len(code) != 1 || knowledgeReadingTestText(code[0]) != original || len(knowledgeReadingTestNodes(t, reading, "script")) != 0 || len(reading.References) != 0 {
		t.Fatal("literal Atom/plain content was interpreted as HTML or lost its first newline")
	}
}

func TestKnowledgeReadingReferenceDeduplication(t *testing.T) {
	limits := knowledgeReadingTestLimits()
	limits.References = 1
	original := []byte(`<base href="javascript:bad"><base href="https://cdn.example.test/root/"><a href="x#part">same</a><a href="x#part">same</a>`)
	reading, err := renderKnowledgeReading(context.Background(), original, "https://page.example.test/a", "text/html", limits)
	if err != nil || len(reading.References) != 1 || reading.References[0].ResolvedURL != "https://cdn.example.test/root/x#part" || strings.Count(string(reading.HTML), `data-knowledge-ref="1"`) != 2 {
		t.Fatalf("stable repeated references failed: %+v %v", reading, err)
	}
	if _, err := renderKnowledgeReading(context.Background(), []byte(`<a href="x">one</a><a href="y">two</a>`), "https://page.example.test/a", "text/html", limits); err == nil {
		t.Fatal("distinct reference exceeded budget")
	}
}

func TestKnowledgeReadingDoesNotFetchResources(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); w.WriteHeader(http.StatusOK) }))
	defer server.Close()
	original := []byte(`<img src="` + server.URL + `/image"><iframe src="` + server.URL + `/frame"></iframe><script src="` + server.URL + `/script"></script><link rel="stylesheet" href="` + server.URL + `/style">`)
	if _, err := renderKnowledgeReading(context.Background(), original, server.URL, "text/html", knowledgeReadingTestLimits()); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 0 {
		t.Fatal("offline renderer made a resource request")
	}
}

func TestKnowledgeReadingRejectsLimitsAndEncoding(t *testing.T) {
	tests := []struct {
		name   string
		body   []byte
		media  string
		mutate func(*knowledgeReadingLimits)
	}{
		{"invalid UTF8", []byte{0xff}, "text/html", nil},
		{"NUL", []byte{'a', 0, 'b'}, "text/html", nil},
		{"input", []byte(strings.Repeat("x", 101)), "text/html", func(l *knowledgeReadingLimits) { l.InputBytes = 100 }},
		{"escaped output", []byte("<p>" + strings.Repeat("&", 1024) + "</p>"), "text/html", func(l *knowledgeReadingLimits) { l.OutputBytes = 4096 }},
		{"2MiB dense output includes envelope", []byte(strings.Repeat("x", 2<<20)), "text/plain", nil},
		{"depth", []byte(strings.Repeat("<div>", 65) + "x" + strings.Repeat("</div>", 65)), "text/html", nil},
		{"nodes", []byte(strings.Repeat("<p>x</p>", 100)), "text/html", func(l *knowledgeReadingLimits) { l.Nodes = 100 }},
		{"unsupported media", []byte("body"), "application/pdf", nil},
		{"invalid limits", []byte("body"), "text/html", func(l *knowledgeReadingLimits) { l.OutputBytes = 0 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			limits := knowledgeReadingTestLimits()
			if test.mutate != nil {
				test.mutate(&limits)
			}
			result, err := renderKnowledgeReading(context.Background(), test.body, "https://page.example.test/a", test.media, limits)
			if err == nil || result != nil {
				t.Fatal("invalid input yielded a partial successful reading")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if result, err := renderKnowledgeReading(ctx, []byte("<p>body</p>"), "https://page.example.test/a", "text/html", knowledgeReadingTestLimits()); !errors.Is(err, context.Canceled) || result != nil {
		t.Fatal("cancelled work published a reading")
	}
}
