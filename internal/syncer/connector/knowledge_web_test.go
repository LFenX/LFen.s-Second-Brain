package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"ragflow/internal/entity"
)

var webTestClass = entity.KnowledgeDirectoryClassification{ComponentSide: "frontend", Language: "typescript"}

func parseWebTest(t *testing.T, kind, media, text string) *KnowledgeWebParseResult {
	t.Helper()
	result, err := NormalizeKnowledgeWebCapture(t.Context(), kind, "https://feed.example.test/base/feed", media, []byte(text), webTestClass)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func rssTest(items string) string {
	return `<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/"><channel>` + items + `</channel></rss>`
}
func webParseCode(t *testing.T, err error, code string) {
	t.Helper()
	var public *KnowledgeWebParseError
	if !errors.As(err, &public) || public.Code != code {
		t.Fatalf("err=%v want=%s", err, code)
	}
}

func TestKnowledgeWebURLContract(t *testing.T) {
	for _, tc := range []struct {
		raw, base, want string
		fragment        bool
	}{
		{"HTTPS://EXAMPLE.test:443", "", "https://example.test/", false},
		{"../a%2Fb?z=%2F&x=1", "https://example.test/base/feed", "https://example.test/a%2Fb?z=%2F&x=1", false},
		{"#part", "https://example.test/a", "https://example.test/a#part", true},
	} {
		got, err := NormalizeKnowledgeWebURL(tc.base, tc.raw, tc.fragment)
		if err != nil || got != tc.want {
			t.Fatalf("url=%q err=%v want=%q", got, err, tc.want)
		}
	}
	for _, raw := range []string{"https://user:pass@example.test/", "https://example.test:444/", "http://example.test:443/", "https://例子.test/", "https://example.test/#fragment", "https://example.test/?API_KEY=x", "https://example.test/?%74oken=x", "file:///etc/passwd", "https://example.test/\nheader", "https://example.test/?a=%zz"} {
		if _, err := NormalizeKnowledgeWebURL("", raw, false); err == nil {
			t.Fatal("unsafe locator accepted", raw)
		}
	}
}

func TestKnowledgeWebRSSIdentityDuplicatesAndFingerprint(t *testing.T) {
	item := `<item><guid> ID:不折叠 </guid><title>合成代码</title><author>作者甲</author><category>z</category><category>a</category><category>z</category><pubDate>same-time</pubDate><content:encoded><![CDATA[<pre><code class="language-ts">const 值 = 1;
  run(值);</code></pre><table><tr><td colspan="2">表</td></tr></table>]]></content:encoded><enclosure url="../file.pdf#p1" type="application/pdf"/></item>`
	r := parseWebTest(t, "web_feed", "application/rss+xml", rssTest(item+item))
	if r.InputEntryCount != 2 || r.DuplicateCount != 1 || len(r.Entries) != 1 {
		t.Fatal("duplicate folding failed")
	}
	e := r.Entries[0]
	if e.Entry.IdentityValue != " ID:不折叠 " || e.Entry.IdentityBasis != "rss_guid" || len(e.Entry.Positions) != 2 || strings.Join(e.Entry.Metadata.Categories, ",") != "a,z" || !e.Entry.Eligible || e.Entry.SourceID != nil || e.Entry.CurrentRevisionID != nil {
		t.Fatal("identity or fixed observation changed", e.Entry)
	}
	if !bytes.Contains(e.Decoded, []byte("\n  run")) || !bytes.Contains(e.Reading, []byte(`colspan="2"`)) || len(e.References) != 1 || *e.References[0].ResolvedURL != "https://feed.example.test/file.pdf#p1" {
		t.Fatal("code table or reference lost")
	}
	for _, change := range []struct{ name, old, new string }{
		{"author", "作者甲", "作者乙"}, {"category", "<category>a</category>", "<category>b</category>"}, {"same timestamp body", "const 值 = 1;", "const 值 = 2;"}, {"raw identity spaces", " ID:不折叠 ", "ID:不折叠"},
	} {
		t.Run(change.name, func(t *testing.T) {
			changed := parseWebTest(t, "web_feed", "application/xml", rssTest(strings.Replace(item, change.old, change.new, 1)))
			if *changed.Entries[0].Entry.SourceInputSHA256 == *e.Entry.SourceInputSHA256 {
				t.Fatal("meaningful change did not revise input")
			}
		})
	}
	_, err := NormalizeKnowledgeWebCapture(t.Context(), "web_feed", "https://feed.example.test/", "application/xml", []byte(rssTest(item+strings.Replace(item, "const 值 = 1;", "const 值 = 2;", 1))), webTestClass)
	webParseCode(t, err, "web_entry_conflict")
	other := `<item><guid>other</guid><description>other text</description></item>`
	reordered := parseWebTest(t, "web_feed", "text/xml", rssTest(other+item))
	for _, product := range reordered.Entries {
		if product.Entry.EntryKey == e.Entry.EntryKey && *product.Entry.SourceInputSHA256 != *e.Entry.SourceInputSHA256 {
			t.Fatal("unrelated order changed input")
		}
	}
}

func TestKnowledgeWebWeakAndMissingIdentities(t *testing.T) {
	r := parseWebTest(t, "web_feed", "application/xml", rssTest(`<item><title>只有标题</title><description>text</description></item><item><title>相同标题</title><link>../page#fragment</link><description>text</description></item>`))
	for _, p := range r.Entries {
		if p.Entry.IdentityBasis == "missing" {
			if p.Entry.Eligible || p.Entry.IneligibleReason == nil || *p.Entry.IneligibleReason != "identity_missing" {
				t.Fatal("missing title invented identity")
			}
		} else if p.Entry.IdentityBasis != "weak_url" || p.Entry.IdentityValue != "https://feed.example.test/page" || !p.Entry.Eligible {
			t.Fatal("weak fallback locator wrong", p.Entry)
		}
	}
	if empty := parseWebTest(t, "web_feed", "application/rss+xml", rssTest("")); len(empty.Entries) != 0 || empty.InputEntryCount != 0 {
		t.Fatal("empty window not represented")
	}
}

func TestKnowledgeWebAtomTypedBodiesAndBase(t *testing.T) {
	feed := `<feed xmlns="http://www.w3.org/2005/Atom" xml:base="https://example.test/root/"><author><name>默认作者</name></author><entry xml:base="a/"><id>Tag:大写 原样</id><title>literal</title><content type="text">&lt;script&gt;literal&lt;/script&gt;</content></entry><entry xml:base="b/"><id>xhtml</id><content type="xhtml" xml:base="c/"><div xmlns="http://www.w3.org/1999/xhtml" xml:base="d/"><pre><code>const 文 = 1;
  run(文);</code></pre><a href="../next#part">next</a></div></content></entry><entry><id>external</id><content src="../article" type="text/html"/></entry><entry><id>summary</id><content src="article" type="text/html"/><summary type="html">&lt;p&gt;摘要&lt;/p&gt;</summary></entry></feed>`
	r := parseWebTest(t, "web_feed", "application/atom+xml", feed)
	byID := map[string]KnowledgeWebEntryContent{}
	for _, e := range r.Entries {
		byID[e.Entry.IdentityValue] = e
	}
	literal := byID["Tag:大写 原样"]
	if literal.Entry.Metadata.Authors[0].Name != "默认作者" || literal.Entry.Representations[0].MediaType != "text/plain" || !bytes.Contains(literal.Reading, []byte("&lt;script&gt;literal&lt;/script&gt;")) {
		t.Fatal("Atom text treated as HTML")
	}
	xhtml := byID["xhtml"]
	if len(xhtml.References) != 1 || *xhtml.References[0].ResolvedURL != "https://example.test/root/b/c/next#part" || !bytes.Contains(xhtml.Reading, []byte("\n  run")) {
		t.Fatalf("nested xml:base or code lost: %+v", xhtml.References)
	}
	external := byID["external"]
	if !external.Entry.Eligible || len(external.Decoded) != 0 || external.Entry.BodyCompleteness != "external_reference" || !bytes.Contains(external.Reading, []byte("仅外部引用")) {
		t.Fatal("external content was fetched or disguised as full")
	}
	summary := byID["summary"]
	if summary.Entry.BodyCompleteness != "external_reference" || string(summary.Decoded) != "<p>摘要</p>" {
		t.Fatal("external summary lost")
	}
}

func TestKnowledgeWebPageRawCanonicalAndSafeReading(t *testing.T) {
	raw := []byte("\xef\xbb\xbf<!doctype html><html><head><base href=\"/assets/\"><link rel=\"canonical\" href=\"../canonical\"><title>页面</title><meta name=\"author\" content=\"作者\"></head><body><script src=\"https://never.example.test/a\">run()</script><img src=\"./image.png#part\" alt=\"图片\"><pre><code>const 名 = 1;\r\n\tgo(名);</code></pre></body></html>")
	before := bytes.Clone(raw)
	r, err := NormalizeKnowledgeWebCapture(t.Context(), "web_page", "https://example.test/page", "text/html", raw, webTestClass)
	if err != nil {
		t.Fatal(err)
	}
	e := r.Entries[0]
	if !bytes.Equal(raw, before) || !bytes.Equal(e.Decoded, raw) || e.Entry.IdentityValue != "page" || *e.Entry.Metadata.CanonicalResolved != "https://example.test/canonical" || e.Entry.Metadata.EffectiveBase != "https://example.test/assets/" || *e.References[0].ResolvedURL != "https://example.test/assets/image.png#part" {
		t.Fatal("raw or base/canonical identity wrong")
	}
	if bytes.Contains(e.Reading, []byte("<script")) || bytes.Contains(e.Reading, []byte("<img")) || !bytes.Contains(e.Reading, []byte("\n\tgo")) {
		t.Fatal("unsafe content or code damage")
	}
	withoutCanonical := bytes.Replace(raw, []byte(`<link rel="canonical" href="../canonical">`), nil, 1)
	other, err := NormalizeKnowledgeWebCapture(t.Context(), "web_page", "https://example.test/page", "text/html", withoutCanonical, webTestClass)
	if err != nil {
		t.Fatal(err)
	}
	if e.Entry.EntryKey != other.Entries[0].Entry.EntryKey || *e.Entry.SourceInputSHA256 == *other.Entries[0].Entry.SourceInputSHA256 {
		t.Fatal("canonical affected identity or not input")
	}
}

func TestKnowledgeWebRejectsMalformedAndBudgets(t *testing.T) {
	for _, tc := range []struct{ name, kind, media, body, code string }{
		{"DTD", "web_feed", "text/xml", `<!DOCTYPE rss [<!ENTITY x "boom">]><rss version="2.0"><channel/></rss>`, "web_parse_rejected"},
		{"external entity", "web_feed", "text/xml", `<!DOCTYPE rss SYSTEM "https://never.example.test/a"><rss version="2.0"><channel/></rss>`, "web_parse_rejected"},
		{"unknown entity", "web_feed", "text/xml", rssTest(`<item><description>&external;</description></item>`), "web_parse_rejected"},
		{"malformed tail", "web_feed", "text/xml", rssTest("") + "broken", "web_parse_rejected"},
		{"RSS1", "web_feed", "text/xml", `<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"/>`, "web_format_unsupported"},
		{"entry only", "web_feed", "application/atom+xml", `<entry xmlns="http://www.w3.org/2005/Atom"/>`, "web_format_unsupported"},
		{"media root mismatch", "web_feed", "application/atom+xml", rssTest(""), "web_format_unsupported"},
		{"non UTF8", "web_feed", "text/xml", string([]byte{0xff, 0xfe}), "web_encoding_unsupported"},
		{"NUL", "web_feed", "text/xml", rssTest("\x00"), "web_encoding_unsupported"},
		{"1001 items", "web_feed", "text/xml", rssTest(strings.Repeat(`<item><description>x</description></item>`, 1001)), "web_budget_exceeded"},
		{"depth", "web_feed", "text/xml", rssTest(strings.Repeat("<x>", 65) + strings.Repeat("</x>", 65)), "web_budget_exceeded"},
		{"title limit", "web_feed", "text/xml", rssTest(`<item><guid>x</guid><title>` + strings.Repeat("a", 129) + `</title><description>x</description></item>`), "web_budget_exceeded"},
		{"raw limit", "web_page", "text/html", strings.Repeat("a", (2<<20)+1), "web_too_large"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeKnowledgeWebCapture(t.Context(), tc.kind, "https://example.test/feed", tc.media, []byte(tc.body), webTestClass)
			webParseCode(t, err, tc.code)
			if got != nil {
				t.Fatal("partial observation escaped failure")
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := NormalizeKnowledgeWebCapture(ctx, "web_feed", "https://example.test/feed", "text/xml", []byte(rssTest("")), webTestClass); !errors.Is(err, context.Canceled) {
		t.Fatal("cancel ignored")
	}
}

func TestKnowledgeWebThousandEntriesAndDeterminism(t *testing.T) {
	var items strings.Builder
	for i := 0; i < 1000; i++ {
		fmt.Fprintf(&items, "<item><guid>entry-%d</guid><description>合成%d</description></item>", i, i)
	}
	first := parseWebTest(t, "web_feed", "text/xml", rssTest(items.String()))
	second := parseWebTest(t, "web_feed", "text/xml", rssTest(items.String()))
	if len(first.Entries) != 1000 || first.InputEntryCount != 1000 || !bytes.Equal(knowledgeWebJSON(first), knowledgeWebJSON(second)) {
		t.Fatal("1000 items not deterministic")
	}
	if _, err := json.Marshal(first); err != nil {
		t.Fatal(err)
	}
}

func TestKnowledgeWebCaptureNodeBudgetCountsEmbeddedHTML(t *testing.T) {
	var items strings.Builder
	for i := 0; i < 700; i++ {
		fmt.Fprintf(&items, "<item><guid>%d</guid><description><![CDATA[%s]]></description></item>", i, strings.Repeat("<span>x</span>", 40))
	}
	result, err := NormalizeKnowledgeWebCapture(t.Context(), "web_feed", "https://example.test/feed", "text/xml", []byte(rssTest(items.String())), webTestClass)
	webParseCode(t, err, "web_budget_exceeded")
	if result != nil {
		t.Fatal("embedded HTML exceeded total nodes but produced observation")
	}
}

func TestKnowledgeWebInactiveBodyAndBaseWarnings(t *testing.T) {
	parsed := parseWebTest(t, "web_page", "text/html", `<html><head><base href="javascript:run()"><base href="https://example.test/a/"><base href="/ignored/"></head><body><script>run()</script></body></html>`)
	if parsed.Entries[0].Entry.Eligible || !slices.Contains(parsed.Warnings, "multiple_html_bases") || !slices.Contains(parsed.Warnings, "invalid_base_ignored") || parsed.Entries[0].Entry.Metadata.EffectiveBase != "https://example.test/a/" {
		t.Fatal("invisible body or base warning wrong")
	}
	atom := parseWebTest(t, "web_feed", "application/atom+xml", `<feed xmlns="http://www.w3.org/2005/Atom" xml:base="javascript:run()"><entry><id>x</id><content type="text">合成</content></entry></feed>`)
	if !slices.Contains(atom.Warnings, "invalid_base_ignored") {
		t.Fatal("invalid XML base silently ignored")
	}
}

func TestKnowledgeWebRelativePageBaseAppliesOnce(t *testing.T) {
	parsed, err := NormalizeKnowledgeWebCapture(t.Context(), "web_page", "https://example.test/path/page", "text/html", []byte(`<html><head><base href="assets/"><link rel="canonical" href="../canonical"></head><body><a href="../guide#part">合成</a></body></html>`), webTestClass)
	if err != nil {
		t.Fatal(err)
	}
	entry := parsed.Entries[0]
	if entry.Entry.Metadata.EffectiveBase != "https://example.test/path/assets/" || *entry.Entry.Metadata.CanonicalResolved != "https://example.test/path/canonical" || len(entry.References) != 1 || *entry.References[0].ResolvedURL != "https://example.test/path/guide#part" {
		t.Fatalf("relative base applied twice: %+v %+v", entry.Entry.Metadata, entry.References)
	}
}
