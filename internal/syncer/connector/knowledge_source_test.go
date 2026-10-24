package connector

import (
	"bytes"
	"fmt"
	"html"
	"strings"
	"testing"
	"time"
)

const preparationFeedURL = "https://synthetic.invalid/rss/feed.xml"

func preparationWebTime() time.Time { return time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC) }

func preparationRSS(items string) []byte {
	return []byte(`<?xml version="1.0"?><rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/"><channel><title>合成订阅</title>` + items + `</channel></rss>`)
}

func preparationRSSItem(id, title, body string) string {
	return `<item><guid>` + id + `</guid><title>` + title + `</title><link>https://synthetic.invalid/articles/one</link><author>合成作者</author><category>前端</category><pubDate>Fri, 23 Oct 2026 08:00:00 GMT</pubDate><description>摘要</description><content:encoded><![CDATA[` + body + `]]></content:encoded><enclosure url="../files/sample.pdf" type="application/pdf"/></item>`
}

func TestKnowledgeSourcePreparationPage(t *testing.T) {
	original := []byte(`<!doctype html><html><head><title>合成组件说明</title><link rel="canonical" href="/canonical/one"/><meta property="article:published_time" content="2026-10-23T08:00:00Z"/></head><body><h1>DataTable</h1><pre><code>const x = 1;
  return x;</code></pre><table><thead><tr><th>名称</th><th>用途</th></tr></thead><tbody><tr><td>sort</td><td>排序</td></tr></tbody></table><a href="../files/readme.pdf" download>附件</a><img src="/images/diagram.png"/><script>alert('synthetic')</script></body></html>`)
	first, err := normalizeKnowledgePage("synthetic-reg", "https://synthetic.invalid/articles/one#top", original, preparationWebTime())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Document.Blob, original) || first.Document.SizeBytes != int64(len(original)) || first.Document.Metadata["original_sha256"] != knowledgeWebHash(original) {
		t.Fatal("original HTML changed")
	}
	if first.OriginalURL != "https://synthetic.invalid/articles/one" || first.CanonicalURL != "https://synthetic.invalid/canonical/one" || first.PublishedAt.IsZero() || first.BodyCompleteness != "unknown" {
		t.Fatalf("provenance lost: %+v", first)
	}
	for _, value := range []string{"DataTable", "const x = 1;", "return x;", "名称", "用途", "sort", "排序"} {
		if !strings.Contains(first.Markdown, value) {
			t.Fatalf("reading view lost %q: %s", value, first.Markdown)
		}
	}
	if !strings.Contains(first.Markdown, "<table>") || !strings.Contains(first.Markdown, "<th>名称</th>") || (!strings.Contains(first.Markdown, "```") && !strings.Contains(first.Markdown, "    const")) {
		t.Fatalf("code/table structure lost: %s", first.Markdown)
	}
	if strings.Contains(first.Markdown, "alert") || len(first.Attachments) != 2 {
		t.Fatalf("unsafe reading or attachment loss: %+v", first)
	}
	if first.Attachments[0].URL != "https://synthetic.invalid/files/readme.pdf" || first.Attachments[1].URL != "https://synthetic.invalid/images/diagram.png" {
		t.Fatalf("attachment references changed: %+v", first.Attachments)
	}
	repeated, err := normalizeKnowledgePage("synthetic-reg", first.OriginalURL, original, preparationWebTime().Add(time.Hour))
	if err != nil || repeated.Document.SourceID != first.Document.SourceID || repeated.Document.Fingerprint != first.Document.Fingerprint {
		t.Fatal("fetch time created a new identity or revision")
	}
	changed := bytes.Replace(original, []byte("合成组件说明"), []byte("更新标题"), 1)
	updated, err := normalizeKnowledgePage("synthetic-reg", first.OriginalURL, changed, preparationWebTime())
	if err != nil || updated.Document.SourceID != first.Document.SourceID || updated.Document.Fingerprint == first.Document.Fingerprint {
		t.Fatal("page update changed identity or lost revision")
	}
	other, err := normalizeKnowledgePage("synthetic-reg", "https://synthetic.invalid/articles/other", original, preparationWebTime())
	if err != nil || other.Document.SourceID == first.Document.SourceID {
		t.Fatal("canonical URL merged distinct source URLs")
	}
}

func TestKnowledgeSourcePreparationRSS(t *testing.T) {
	body := `<h2>组件</h2><pre><code>const label = "中文";
  render(label);</code></pre><table><tr><th>prop</th><th>type</th></tr><tr><td>rows</td><td>Row[]</td></tr></table>`
	item := preparationRSSItem("stable-one", "合成标题", body)
	original := preparationRSS(item)
	first, err := normalizeKnowledgeFeed("synthetic-reg", preparationFeedURL, original, preparationWebTime())
	if err != nil || len(first.Records) != 1 {
		t.Fatalf("RSS: %+v %v", first, err)
	}
	record := first.Records[0]
	if !bytes.Equal(first.OriginalFeed, original) || !bytes.Equal(record.Document.Blob, []byte(body)) || record.BodyCompleteness != "unknown" || record.PublishedAt.IsZero() {
		t.Fatal("RSS originals/provenance lost")
	}
	if len(record.Attachments) != 1 || record.Attachments[0].URL != "https://synthetic.invalid/files/sample.pdf" || record.Document.Metadata["author"] != "合成作者" {
		t.Fatalf("RSS metadata lost: %+v", record)
	}
	if !strings.Contains(record.Markdown, "<td>Row[]</td>") || !strings.Contains(record.Markdown, "<th>type</th>") {
		t.Fatalf("RSS table flattened: %s", record.Markdown)
	}
	repeated, err := normalizeKnowledgeFeed("synthetic-reg", preparationFeedURL, preparationRSS(item+item), preparationWebTime().Add(time.Hour))
	if err != nil || len(repeated.Records) != 1 || repeated.Records[0].Document.Fingerprint != record.Document.Fingerprint {
		t.Fatal("identical duplicate/refetch published another candidate")
	}
	updated, err := normalizeKnowledgeFeed("synthetic-reg", preparationFeedURL, preparationRSS(preparationRSSItem("stable-one", "更新标题", body+"<p>更新</p>")), preparationWebTime())
	if err != nil || updated.Records[0].Document.SourceID != record.Document.SourceID || updated.Records[0].Document.Fingerprint == record.Document.Fingerprint {
		t.Fatal("RSS revision identity changed")
	}
	other, err := normalizeKnowledgeFeed("synthetic-reg", "https://synthetic.invalid/other/feed.xml", original, preparationWebTime())
	if err != nil || other.Records[0].Document.SourceID == record.Document.SourceID {
		t.Fatal("different feed sources merged by guid")
	}
	other, err = normalizeKnowledgeFeed("other-registration", preparationFeedURL, original, preparationWebTime())
	if err != nil || other.Records[0].Document.SourceID == record.Document.SourceID {
		t.Fatal("different registrations merged")
	}
	if _, err := normalizeKnowledgeFeed("synthetic-reg", preparationFeedURL, preparationRSS(item+preparationRSSItem("stable-one", "conflict", body)), preparationWebTime()); err == nil {
		t.Fatal("conflicting duplicate silently discarded")
	}
	// A bounded feed window is an observation, never a complete deletion list.
	window, err := normalizeKnowledgeFeed("synthetic-reg", preparationFeedURL, preparationRSS(item+preparationRSSItem("stable-two", "another", body)), preparationWebTime())
	if err != nil || len(window.Records) != 2 {
		t.Fatal(err)
	}
	rolled, err := normalizeKnowledgeFeed("synthetic-reg", preparationFeedURL, original, preparationWebTime())
	if err != nil || len(rolled.Records) != 1 || rolled.Records[0].Document.Fingerprint != record.Document.Fingerprint {
		t.Fatal("rolling feed changed retained item")
	}
	// Existing native behavior is unchanged and is not the prepared adapter.
	entries, err := parseRSSFeed(original)
	if err != nil || strings.Contains(string(entries[0].toSourceDocument(preparationFeedURL).Blob), "<table>") {
		t.Fatal("native baseline unexpectedly changed")
	}
	if entries[0].toSourceDocument(preparationFeedURL).SourceID != entries[0].toSourceDocument("https://synthetic.invalid/other").SourceID {
		t.Fatal("native RSS identity unexpectedly changed")
	}
}

func TestKnowledgeSourcePreparationAtom(t *testing.T) {
	for _, kind := range []string{"html", "xhtml", "text", "summary", "external_reference"} {
		t.Run(kind, func(t *testing.T) {
			markup := `<pre><code>const ok = true;</code></pre><table><tr><th>key</th><th>value</th></tr><tr><td>a</td><td>b</td></tr></table>`
			content := `<content type="html">` + html.EscapeString(markup) + `</content>`
			want := "unknown"
			switch kind {
			case "xhtml":
				content = `<content type="xhtml"><div xmlns="http://www.w3.org/1999/xhtml">` + markup + `</div></content>`
			case "text":
				content = `<content type="text">合成纯文本 &lt;not-html&gt;</content>`
			case "summary":
				content = `<summary type="html">&lt;p&gt;合成摘要&lt;/p&gt;</summary>`
				want = "summary"
			case "external_reference":
				content = `<content src="/files/full.pdf" type="application/pdf"/><summary>合成摘要</summary>`
				want = "external_reference"
			}
			original := []byte(`<feed xmlns="http://www.w3.org/2005/Atom"><title>合成订阅</title><entry><id>urn:synthetic:one</id><title>合成条目</title><link rel="alternate" href="/articles/one"/><link rel="enclosure" href="/files/sample.pdf" type="application/pdf"/><published>2026-10-23T08:00:00Z</published><updated>2026-10-24T08:00:00Z</updated>` + content + `</entry></feed>`)
			observed, err := normalizeKnowledgeFeed("synthetic-reg", preparationFeedURL, original, preparationWebTime())
			if err != nil || len(observed.Records) != 1 {
				t.Fatalf("Atom: %+v %v", observed, err)
			}
			record := observed.Records[0]
			if record.BodyCompleteness != want || record.PublishedAt.IsZero() || record.Document.UpdatedAt.IsZero() || len(record.Attachments) < 1 {
				t.Fatalf("Atom provenance lost: %+v", record)
			}
			if (kind == "html" || kind == "xhtml") && (!strings.Contains(string(record.Document.Blob), "<table>") || !strings.Contains(record.Markdown, "const ok = true;")) {
				t.Fatalf("Atom markup lost: %s", record.Document.Blob)
			}
			if kind == "text" && !strings.Contains(string(record.Document.Blob), "&lt;not-html&gt;") {
				t.Fatal("Atom text interpreted as markup")
			}
			if kind == "external_reference" && len(record.Attachments) != 2 {
				t.Fatal("external content reference fetched or lost")
			}
		})
	}
}

func TestKnowledgeSourcePreparationWebRejections(t *testing.T) {
	for _, value := range []string{"file:///private", "javascript:alert(1)", "https://user:password@synthetic.invalid/private", "data:text/html,hello"} {
		t.Run(value, func(t *testing.T) {
			if _, err := normalizeKnowledgePage("synthetic-reg", value, []byte("<p>synthetic</p>"), preparationWebTime()); err == nil {
				t.Fatal("unsafe URL admitted")
			}
		})
	}
	for _, payload := range [][]byte{[]byte(`<rss><channel>`), preparationRSS(`<item><title>title-only</title><description>summary</description></item>`), []byte(`<html/>`)} {
		if _, err := normalizeKnowledgeFeed("synthetic-reg", preparationFeedURL, payload, preparationWebTime()); err == nil {
			t.Fatalf("invalid feed accepted: %s", payload)
		}
	}
	for _, payload := range [][]byte{{0xff}, []byte("<p>nul\x00</p>"), bytes.Repeat([]byte("x"), (8<<20)+1), []byte(`<a href="javascript:alert(1)">unsafe</a>`)} {
		if _, err := normalizeKnowledgePage("synthetic-reg", "https://synthetic.invalid/page", payload, preparationWebTime()); err == nil {
			t.Fatal("invalid page accepted")
		}
	}
	summary := preparationRSS(`<item><guid>summary-only</guid><description>合成摘要</description></item>`)
	observed, err := normalizeKnowledgeFeed("synthetic-reg", preparationFeedURL, summary, preparationWebTime())
	if err != nil || observed.Records[0].BodyCompleteness != "summary" {
		t.Fatal("summary was presented as a complete article")
	}
	// Reference normalization has no network capability; .invalid is never fetched.
	if got, err := knowledgeWebURL(preparationFeedURL, "../files/a.pdf#page=2"); err != nil || got != "https://synthetic.invalid/files/a.pdf" {
		t.Fatal(fmt.Sprintf("reference: %s %v", got, err))
	}
}
