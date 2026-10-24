package connector

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/PuerkitoBio/goquery"
)

// Private, offline preparation only: no connector registration, HTTP fetch,
// pruning, C2 DTO, source publication or model execution is added here.
type knowledgeWebAttachment struct {
	URL       string
	MediaType string
}

type knowledgeWebRecord struct {
	Document         SourceDocument
	OriginalURL      string
	CanonicalURL     string
	PublishedAt      time.Time
	FetchedAt        time.Time
	BodyCompleteness string
	Markdown         string
	Attachments      []knowledgeWebAttachment
}

type knowledgeFeedObservation struct {
	OriginalFeed []byte
	FeedSHA256   string
	FeedURL      string
	Records      []knowledgeWebRecord
}

type knowledgeFeedLink struct {
	URL       string `xml:"url,attr"`
	Href      string `xml:"href,attr"`
	Rel       string `xml:"rel,attr"`
	MediaType string `xml:"type,attr"`
}

type knowledgeAtomBody struct {
	Type  string `xml:"type,attr"`
	Src   string `xml:"src,attr"`
	Inner string `xml:",innerxml"`
}

type knowledgeFeedExtras struct {
	XMLName xml.Name
	Channel struct {
		Items []struct {
			Published string              `xml:"pubDate"`
			Encoded   string              `xml:"encoded"`
			Links     []knowledgeFeedLink `xml:"enclosure"`
		} `xml:"item"`
	} `xml:"channel"`
	Entries []struct {
		Published string              `xml:"published"`
		Content   knowledgeAtomBody   `xml:"content"`
		Summary   knowledgeAtomBody   `xml:"summary"`
		Links     []knowledgeFeedLink `xml:"link"`
	} `xml:"entry"`
}

// normalizeKnowledgeFeed reuses the native RSS/Atom parser for entry fields and
// adds raw markup/provenance that the native plain-text document does not keep.
func normalizeKnowledgeFeed(registrationKey, feedURL string, original []byte, fetchedAt time.Time) (*knowledgeFeedObservation, error) {
	feedURL, err := knowledgeWebURL("", feedURL)
	if err != nil || strings.TrimSpace(registrationKey) == "" || len(registrationKey) > 64 || fetchedAt.IsZero() || len(original) > maxRSSFeedSize {
		return nil, fmt.Errorf("invalid feed preparation scope or payload")
	}
	entries, err := parseRSSFeed(original)
	if err != nil {
		return nil, err
	}
	if len(entries) > 10000 {
		return nil, fmt.Errorf("feed entry preparation limit exceeded")
	}
	var extras knowledgeFeedExtras
	if err := xml.Unmarshal(original, &extras); err != nil {
		return nil, err
	}
	result := &knowledgeFeedObservation{OriginalFeed: bytes.Clone(original), FeedSHA256: knowledgeWebHash(original), FeedURL: feedURL, Records: []knowledgeWebRecord{}}
	seen := map[string]string{}
	for index, entry := range entries {
		itemKey := "id:" + entry.id
		if entry.id == "" {
			if entry.link == "" {
				return nil, fmt.Errorf("entry %d lacks a stable id or URL; title is not an identity", index)
			}
			link, err := knowledgeWebURL(feedURL, entry.link)
			if err != nil {
				return nil, err
			}
			itemKey = "url:" + link
		}
		pageURL := feedURL
		if entry.link != "" {
			pageURL, err = knowledgeWebURL(feedURL, entry.link)
			if err != nil {
				return nil, err
			}
		}
		body, completeness := firstNonEmpty(entry.content, entry.summary), "summary"
		published := time.Time{}
		attachments := []knowledgeWebAttachment{}
		if extras.XMLName.Local == "feed" {
			if index >= len(extras.Entries) {
				return nil, fmt.Errorf("Atom entry provenance mismatch")
			}
			extra := extras.Entries[index]
			published = parseFeedTime(extra.Published)
			content := extra.Content
			if strings.TrimSpace(content.Inner) != "" {
				completeness = "unknown"
			} else {
				content = extra.Summary
			}
			body, err = knowledgeAtomHTML(content)
			if err != nil {
				return nil, err
			}
			if extra.Content.Src != "" {
				reference, err := knowledgeWebURL(pageURL, extra.Content.Src)
				if err != nil {
					return nil, err
				}
				attachments = append(attachments, knowledgeWebAttachment{URL: reference, MediaType: extra.Content.Type})
				completeness = "external_reference"
			}
			for _, link := range extra.Links {
				if link.Rel == "enclosure" {
					reference, err := knowledgeWebURL(pageURL, link.Href)
					if err != nil {
						return nil, err
					}
					attachments = append(attachments, knowledgeWebAttachment{URL: reference, MediaType: link.MediaType})
				}
			}
		} else {
			if index >= len(extras.Channel.Items) {
				return nil, fmt.Errorf("RSS entry provenance mismatch")
			}
			extra := extras.Channel.Items[index]
			published = parseFeedTime(extra.Published)
			if strings.TrimSpace(extra.Encoded) != "" {
				completeness = "unknown"
			}
			for _, link := range extra.Links {
				reference, err := knowledgeWebURL(pageURL, link.URL)
				if err != nil {
					return nil, err
				}
				attachments = append(attachments, knowledgeWebAttachment{URL: reference, MediaType: link.MediaType})
			}
		}
		record, err := prepareKnowledgeWebRecord(registrationKey, feedURL+"\x00"+itemKey, pageURL, entry.title, []byte(body), published, entry.updatedAt, fetchedAt, completeness, attachments)
		if err != nil {
			return nil, err
		}
		record.Document.Metadata["feed_url"] = feedURL
		record.Document.Metadata["feed_sha256"] = result.FeedSHA256
		record.Document.Metadata["item_key"] = itemKey
		record.Document.Metadata["source_locator"] = fmt.Sprintf("entry[%d]", index+1)
		if entry.author != "" {
			record.Document.Metadata["author"] = entry.author
		}
		if len(entry.categories) > 0 {
			record.Document.Metadata["categories"] = slices.Clone(entry.categories)
		}
		if prior, exists := seen[record.Document.SourceID]; exists {
			if prior != record.Document.Fingerprint {
				return nil, fmt.Errorf("conflicting duplicate feed item")
			}
			continue
		}
		seen[record.Document.SourceID] = record.Document.Fingerprint
		result.Records = append(result.Records, *record)
	}
	slices.SortFunc(result.Records, func(a, b knowledgeWebRecord) int { return strings.Compare(a.Document.SourceID, b.Document.SourceID) })
	return result, nil
}

func normalizeKnowledgePage(registrationKey, pageURL string, original []byte, fetchedAt time.Time) (*knowledgeWebRecord, error) {
	pageURL, err := knowledgeWebURL("", pageURL)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(registrationKey) == "" || len(registrationKey) > 64 || fetchedAt.IsZero() {
		return nil, fmt.Errorf("invalid page preparation scope")
	}
	return prepareKnowledgeWebRecord(registrationKey, pageURL, pageURL, "", original, time.Time{}, time.Time{}, fetchedAt, "unknown", nil)
}

func prepareKnowledgeWebRecord(registrationKey, itemKey, pageURL, title string, original []byte, publishedAt, updatedAt, fetchedAt time.Time, completeness string, attachments []knowledgeWebAttachment) (*knowledgeWebRecord, error) {
	if len(original) > 8<<20 || !utf8.Valid(original) || bytes.IndexByte(original, 0) >= 0 {
		return nil, fmt.Errorf("invalid HTML preparation payload")
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(original))
	if err != nil {
		return nil, err
	}
	canonical := pageURL
	if value, exists := doc.Find(`link[rel="canonical"]`).First().Attr("href"); exists {
		canonical, err = knowledgeWebURL(pageURL, value)
		if err != nil {
			return nil, err
		}
	}
	if title == "" {
		title = strings.TrimSpace(doc.Find("title").First().Text())
	}
	if title == "" {
		title = pageURL
	}
	if publishedAt.IsZero() {
		value, _ := doc.Find(`meta[property="article:published_time"]`).Attr("content")
		publishedAt = parseFeedTime(value)
	}
	// Markdown is a derived reading view, not an HTML sanitizer. Original bytes
	// remain unchanged; a future UI must enforce its own safe rendering policy.
	doc.Find("script,style,noscript").Remove()
	attachments = slices.Clone(attachments)
	var linkErr error
	doc.Find("a[href],img[src]").Each(func(_ int, selection *goquery.Selection) {
		if linkErr != nil {
			return
		}
		attribute := "href"
		if goquery.NodeName(selection) == "img" {
			attribute = "src"
		}
		value, _ := selection.Attr(attribute)
		absolute, err := knowledgeWebURL(pageURL, value)
		if err != nil {
			linkErr = err
			return
		}
		selection.SetAttr(attribute, absolute)
		_, download := selection.Attr("download")
		if attribute == "src" || download {
			media, _ := selection.Attr("type")
			attachments = append(attachments, knowledgeWebAttachment{URL: absolute, MediaType: media})
		}
	})
	if linkErr != nil {
		return nil, linkErr
	}
	slices.SortFunc(attachments, func(a, b knowledgeWebAttachment) int {
		return strings.Compare(a.URL+"\x00"+a.MediaType, b.URL+"\x00"+b.MediaType)
	})
	attachments = slices.Compact(attachments)
	if attachments == nil {
		attachments = []knowledgeWebAttachment{}
	}
	markup, err := doc.Find("body").Html()
	if err != nil {
		return nil, err
	}
	converter := newMarkdownConverter()
	// Keep table structure, including merged cells, as embedded HTML. Importing
	// the converter's optional plugin package would add frontmatter dependencies.
	var tableErr error
	converter.AddRules(md.Rule{Filter: []string{"table"}, Replacement: func(_ string, selection *goquery.Selection, _ *md.Options) *string {
		markup, err := goquery.OuterHtml(selection)
		if err != nil {
			tableErr = err
			return nil
		}
		markup = "\n\n" + markup + "\n\n"
		return &markup
	}})
	markdown, err := converter.ConvertString(markup)
	if err != nil {
		return nil, err
	}
	if tableErr != nil {
		return nil, tableErr
	}
	record := &knowledgeWebRecord{OriginalURL: pageURL, CanonicalURL: canonical, PublishedAt: publishedAt.UTC(), FetchedAt: fetchedAt.UTC(), BodyCompleteness: completeness, Markdown: markdown, Attachments: attachments}
	identity, _ := json.Marshal([]string{"knowledge-source-preparation", registrationKey, itemKey})
	revision, _ := json.Marshal(struct {
		URL, Canonical, Title, Completeness string
		Original                            []byte
		Attachments                         []knowledgeWebAttachment
		PublishedAt, UpdatedAt              time.Time
	}{pageURL, canonical, title, completeness, original, attachments, publishedAt.UTC(), updatedAt.UTC()})
	record.Document = SourceDocument{SourceID: "prep:" + knowledgeWebHash(identity), SemanticIdentifier: title, Extension: ".html", Blob: bytes.Clone(original), UpdatedAt: updatedAt.UTC(), SizeBytes: int64(len(original)), Fingerprint: knowledgeWebHash(revision), Metadata: map[string]any{"original_url": pageURL, "canonical_url": canonical, "body_completeness": completeness, "original_sha256": knowledgeWebHash(original)}}
	return record, nil
}

func knowledgeAtomHTML(body knowledgeAtomBody) (string, error) {
	switch body.Type {
	case "xhtml":
		return body.Inner, nil
	case "html":
		return html.UnescapeString(body.Inner), nil
	case "", "text":
		return html.EscapeString(html.UnescapeString(body.Inner)), nil
	default:
		if body.Src != "" {
			return "", nil
		}
		return "", fmt.Errorf("unsupported inline Atom content type")
	}
}

func knowledgeWebURL(base, value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return "", err
	}
	if base != "" {
		parent, err := url.Parse(base)
		if err != nil {
			return "", err
		}
		parsed = parent.ResolveReference(parsed)
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil || parsed.Opaque != "" {
		return "", fmt.Errorf("only HTTP(S) references without credentials are supported")
	}
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Fragment = ""
	return parsed.String(), nil
}

func knowledgeWebHash(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}
