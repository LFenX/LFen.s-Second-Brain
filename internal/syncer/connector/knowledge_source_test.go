package connector

import (
	"bytes"
	"strings"
	"testing"
)

func TestKnowledgeWebControlledReadingAndNativeRSS(t *testing.T) {
	raw := []byte(`<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/"><channel><item><guid>fixed</guid><title>合成表格</title><content:encoded><![CDATA[<pre><code>const 甲 = 1;
  use(甲);</code></pre><table><tr><td>单元格</td></tr></table>]]></content:encoded></item></channel></rss>`)
	native, err := parseRSSFeed(raw)
	if err != nil || len(native) != 1 {
		t.Fatal(err)
	}
	controlled, err := NormalizeKnowledgeWebCapture(t.Context(), "web_feed", "https://example.test/feed", "application/rss+xml", raw, webTestClass)
	if err != nil || len(controlled.Entries) != 1 {
		t.Fatal(err)
	}
	if bytes.Contains(native[0].toSourceDocument("https://example.test/feed").Blob, []byte("<table>")) {
		t.Fatal("generic native RSS behavior changed")
	}
	if !bytes.Contains(controlled.Entries[0].Decoded, []byte("<table>")) || !strings.Contains(string(controlled.Entries[0].Reading), "\n  use(甲);") {
		t.Fatal("controlled snapshot lost structure")
	}
}
