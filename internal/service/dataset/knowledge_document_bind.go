package dataset

import (
	"encoding/json"
	"sort"
	"unicode/utf8"

	"ragflow/internal/entity"
	parser "ragflow/internal/parser/parser"
)

type documentBound struct {
	Representation entity.KnowledgeDocumentRepresentation
	Manifest       entity.KnowledgeDocumentStructureManifest
	Nodes          []entity.KnowledgeDocumentNodeRecord
	ReadingText    string
}

func documentNaturalKey(ref entity.KnowledgeDocumentNaturalRef) (string, error) {
	return entity.KnowledgeLifecycleDigest(ref)
}

func documentRangeValid(r entity.KnowledgeDocumentReadingRange, text string) bool {
	return r.StartByte >= 0 && r.EndByte >= r.StartByte && r.EndByte <= int64(len(text)) && utf8.ValidString(text[r.StartByte:r.EndByte]) && directorySHA([]byte(text[r.StartByte:r.EndByte])) == r.TextSHA256
}

// Binding independently checks the bounded child result before committing any
// identities. The manifest uses natural references, so derived IDs cannot cycle.
func documentBind(owner, domain string, fixed *documentInputs, result parser.DocumentStructureResult) (*documentBound, error) {
	fail := func() (*documentBound, error) { return nil, documentError(409, "document_integrity") }
	if !utf8.ValidString(result.ReadingText) || len(result.ReadingText) > 4<<20 || len(result.Payloads) < 1 || len(result.Payloads) > 8192 || result.Quality != "complete" || result.TextCoverage != "supported_profile_blocks" {
		return fail()
	}
	manifest := entity.KnowledgeDocumentStructureManifest{Nodes: []entity.KnowledgeDocumentManifestEntry{}}
	natural := make(map[string]int, len(result.Payloads))
	depths := make([]int, len(result.Payloads))
	parentIndexes := make([]int, len(result.Payloads))
	ordinals := map[int]int64{}
	passages := []entity.KnowledgeDocumentReadingRange{}
	unsupported := int64(0)
	pages, slides := int64(0), int64(0)
	for i, payload := range result.Payloads {
		sha, err := entity.KnowledgeDocumentPayloadDigest(payload)
		if err != nil {
			return fail()
		}
		if !documentRangeValid(payload.ReadingRange, result.ReadingText) {
			return fail()
		}
		if payload.OwnReadingRange != nil && (!documentRangeValid(*payload.OwnReadingRange, result.ReadingText) || payload.OwnReadingRange.StartByte < payload.ReadingRange.StartByte || payload.OwnReadingRange.EndByte > payload.ReadingRange.EndByte) {
			return fail()
		}
		ref := entity.KnowledgeDocumentNaturalRef{Kind: payload.Kind, Locator: payload.Locator}
		key, err := documentNaturalKey(ref)
		if err != nil {
			return fail()
		}
		if _, duplicate := natural[key]; duplicate {
			return fail()
		}
		parentIndexes[i] = -1
		if i == 0 {
			if payload.Kind != "document" || payload.ParentRef != nil || payload.Ordinal != 0 || payload.ReadingRange.StartByte != 0 || payload.ReadingRange.EndByte != int64(len(result.ReadingText)) {
				return fail()
			}
		} else {
			if payload.Kind == "document" || payload.ParentRef == nil {
				return fail()
			}
			parentKey, err := documentNaturalKey(*payload.ParentRef)
			if err != nil {
				return fail()
			}
			parent, ok := natural[parentKey]
			if !ok {
				return fail()
			}
			parentIndexes[i] = parent
			depths[i] = depths[parent] + 1
			if depths[i] > 16 {
				return fail()
			}
			p := result.Payloads[parent]
			if payload.ReadingRange.StartByte < p.ReadingRange.StartByte || payload.ReadingRange.EndByte > p.ReadingRange.EndByte || payload.Ordinal != ordinals[parent] {
				return fail()
			}
			ordinals[parent]++
			// A final-tree preorder cannot return into a subtree already left.
			if i > 1 {
				ancestor := i - 1
				for ancestor >= 0 && depths[ancestor] > depths[parent] {
					ancestor = parentIndexes[ancestor]
				}
				if ancestor != parent {
					return fail()
				}
			}
		}
		if payload.Kind == "passage" {
			if payload.ReadingRange.EndByte-payload.ReadingRange.StartByte > 8192 {
				return fail()
			}
			passages = append(passages, payload.ReadingRange)
		}
		if payload.Kind == "unsupported_block" {
			unsupported++
		}
		if payload.Kind == "page" {
			pages++
			if payload.Locator.Scheme != "pdf_page_v1" || payload.Locator.PageNumber != pages || parentIndexes[i] != 0 {
				return fail()
			}
		}
		if payload.Kind == "slide" {
			slides++
			if payload.OriginalPosition.Kind != "pptx_slide" || payload.OriginalPosition.SlideNumber != slides || parentIndexes[i] != 0 {
				return fail()
			}
		}
		manifest.Nodes = append(manifest.Nodes, entity.KnowledgeDocumentManifestEntry{NaturalRef: ref, NodePayloadSHA256: sha})
		natural[key] = i
	}
	if unsupported != result.UnsupportedBlockCount {
		return fail()
	}
	switch fixed.Profile.Format {
	case "pdf":
		if result.PageCount == nil || *result.PageCount != pages || pages < 1 || pages > 128 || slides != 0 || result.SlideCount != nil {
			return fail()
		}
	case "pptx":
		if result.SlideCount == nil || *result.SlideCount != slides || slides < 1 || slides > 128 || pages != 0 || result.PageCount != nil {
			return fail()
		}
	case "docx":
		if result.PageCount != nil || result.SlideCount != nil || pages != 0 || slides != 0 {
			return fail()
		}
	default:
		return fail()
	}
	sort.Slice(passages, func(i, j int) bool { return passages[i].StartByte < passages[j].StartByte })
	var cursor int64
	for _, p := range passages {
		if p.StartByte != cursor || p.EndByte <= p.StartByte {
			return fail()
		}
		cursor = p.EndByte
	}
	if cursor != int64(len(result.ReadingText)) {
		return fail()
	}
	manifestSHA, err := entity.KnowledgeDocumentManifestDigest(manifest.Nodes)
	if err != nil {
		return fail()
	}
	rep := entity.KnowledgeDocumentRepresentation{OwnerID: owner, DomainID: domain, Original: fixed.Original, SourceSnapshot: fixed.Snapshot, FoundationPolicyVersion: fixed.FoundationPolicyVersion, DocumentPolicyVersion: fixed.DocumentPolicyVersion, Format: fixed.Profile.Format, ProfileID: fixed.Profile.ProfileID, Configuration: fixed.Profile.Configuration, Producer: fixed.Profile.Producer, ProducerSHA256: fixed.Profile.ProducerSHA256, Quality: "complete", NodeCount: int64(len(result.Payloads)), PageCount: result.PageCount, SlideCount: result.SlideCount, UnsupportedBlockCount: unsupported, TextCoverage: "supported_profile_blocks", CoordinateSpace: "derived_utf8", ReadingFormat: entity.KnowledgeDocumentReadingFormat, ReadingTextSHA256: directorySHA([]byte(result.ReadingText)), ReadingByteLength: int64(len(result.ReadingText)), StructureManifestSHA256: manifestSHA, LexicalIndex: "not_created", VectorIndex: "not_created"}
	repID, err := entity.KnowledgeDocumentRepresentationDigest(rep)
	if err != nil {
		return fail()
	}
	rep.RepresentationID = repID
	bound := &documentBound{Representation: rep, Manifest: manifest, Nodes: []entity.KnowledgeDocumentNodeRecord{}, ReadingText: result.ReadingText}
	ids := make([]string, len(result.Payloads))
	for i, payload := range result.Payloads {
		id, err := entity.KnowledgeDocumentNodeDigest(repID, fixed.Original.SourceRevisionID, manifest.Nodes[i].NodePayloadSHA256)
		if err != nil {
			return fail()
		}
		ids[i] = id
		var parent *string
		parentID := ""
		if parentIndexes[i] >= 0 {
			parentID = ids[parentIndexes[i]]
			parent = &parentID
		} else {
			bound.Representation.RootDocumentNodeID = id
		}
		node := entity.KnowledgeDocumentNode{RepresentationID: repID, DocumentNodeID: id, SourceID: fixed.Original.SourceID, SourceRevisionID: fixed.Original.SourceRevisionID, OriginalSHA256: fixed.Original.SHA256, ProducerSHA256: fixed.Profile.ProducerSHA256, ParentID: parent, NodePayloadSHA256: manifest.Nodes[i].NodePayloadSHA256, Payload: payload}
		encoded, err := json.Marshal(node)
		if err != nil || len(encoded) > 240<<10 {
			return fail()
		}
		bound.Nodes = append(bound.Nodes, entity.KnowledgeDocumentNodeRecord{ID: id, OwnerID: owner, DomainID: domain, RepresentationID: repID, ParentID: parentID, Ordinal: payload.Ordinal, Payload: string(encoded)})
	}
	return bound, nil
}
