package entity

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

const KnowledgeParsedProtocol = "content_parser/0.2.0"
const KnowledgeParsedAssembly = "nonempty_items_lf/0.2.0"

type KnowledgeParsedMember struct {
	Kind       string                `json:"kind"`
	LocalIndex int64                 `json:"local_index"`
	Range      KnowledgeContentRange `json:"range"`
	FirstRow   *int64                `json:"first_row"`
	EndRow     *int64                `json:"end_row"`
}

// Local indices form the identity preimage; fixed references are bound afterwards.
type KnowledgeParsedNodePayload struct {
	LocalIndex          int64                              `json:"local_index"`
	ParentLocalIndex    *int64                             `json:"parent_local_index"`
	Kind                string                             `json:"kind"`
	Name                string                             `json:"name"`
	Ordinal             int64                              `json:"ordinal"`
	Locator             KnowledgeContentLocator            `json:"locator"`
	OriginalPosition    *KnowledgeDocumentOriginalPosition `json:"original_position"`
	CoordinatePrecision string                             `json:"coordinate_precision"`
	Text                KnowledgeContentText               `json:"text"`
	Attributes          KnowledgeContentAttributes         `json:"attributes"`
	Members             []KnowledgeParsedMember            `json:"members"`
}

type KnowledgeParsedNode struct {
	RepresentationID string                     `json:"representation_id"`
	NodeID           string                     `json:"node_id"`
	PayloadSHA256    string                     `json:"payload_sha256"`
	Payload          KnowledgeParsedNodePayload `json:"payload"`
}

type KnowledgeParsedManifestEntry struct {
	LocalIndex       int64  `json:"local_index"`
	ParentLocalIndex *int64 `json:"parent_local_index"`
	PayloadSHA256    string `json:"payload_sha256"`
}

type KnowledgeParsedRepresentation struct {
	Version              string                            `json:"projection_version"`
	Protocol             string                            `json:"protocol"`
	Source               KnowledgeContentSourceRef         `json:"source_ref"`
	Ref                  KnowledgeContentRepresentationRef `json:"representation_ref"`
	RootRef              KnowledgeContentNodeRef           `json:"root_ref"`
	ReadingTextSHA256    string                            `json:"reading_text_sha256"`
	ReadingByteLength    int64                             `json:"reading_byte_length"`
	ManifestSHA256       string                            `json:"manifest_sha256"`
	NodeCount            int64                             `json:"node_count"`
	State                string                            `json:"state"`
	UnsupportedNodeCount int64                             `json:"unsupported_node_count"`
	UnavailableNodeCount int64                             `json:"unavailable_node_count"`
	Assembly             string                            `json:"assembly"`
	RangePlanVersion     string                            `json:"range_plan_version"`
	MaxUnitBytes         int64                             `json:"max_unit_bytes"`
}

type KnowledgeParsedRepresentationQuery struct {
	Source KnowledgeContentSourceRef `json:"source_ref"`
	Cursor string                    `json:"cursor"`
	Limit  int                       `json:"limit"`
}

type KnowledgeParsedRepresentationPage struct {
	Version    string                    `json:"projection_version"`
	Source     KnowledgeContentSourceRef `json:"source_ref"`
	Items      []KnowledgeContentTarget  `json:"items"`
	NextCursor *string                   `json:"next_cursor"`
}

// Decimal producer positions remain typed JSON. The existing typed-LP identity
// codec receives only the resulting SHA, strings, nulls and bounded integers.
func KnowledgeParsedPayloadDigest(payload KnowledgeParsedNodePayload) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	if len(raw) > 32<<20 {
		return "", fmt.Errorf("parsed payload capacity exceeded")
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func KnowledgeParsedManifestDigest(manifest []KnowledgeParsedManifestEntry) (string, error) {
	if len(manifest) < 1 || len(manifest) > 32768 {
		return "", fmt.Errorf("invalid parsed manifest size")
	}
	for i, item := range manifest {
		if item.LocalIndex != int64(i) || !knowledgeDocumentHex(item.PayloadSHA256, 64) || i == 0 && item.ParentLocalIndex != nil || i > 0 && (item.ParentLocalIndex == nil || *item.ParentLocalIndex < 0 || *item.ParentLocalIndex >= int64(i)) {
			return "", fmt.Errorf("invalid parsed manifest entry")
		}
	}
	return KnowledgeLifecycleDigest(map[string]any{"kind": "parsed-manifest/0.2.0", "entries": manifest})
}

func KnowledgeParsedRepresentationDigest(rep KnowledgeParsedRepresentation) (string, error) {
	if rep.Version != KnowledgeContentVersion || rep.Protocol != KnowledgeParsedProtocol || rep.Source.Validate(rep.Source.DomainID) != nil || rep.Ref.Kind != "parsed_representation" || rep.Ref.ProducerSHA256 == nil || !knowledgeDocumentHex(*rep.Ref.ProducerSHA256, 64) || !knowledgeDocumentHex(rep.ReadingTextSHA256, 64) || !knowledgeDocumentHex(rep.ManifestSHA256, 64) || rep.ReadingByteLength < 0 || rep.ReadingByteLength > 4<<20 || rep.MaxUnitBytes < 8 || rep.MaxUnitBytes > 8192 || rep.NodeCount < 1 || rep.NodeCount > 32768 || rep.Assembly != KnowledgeParsedAssembly || rep.RangePlanVersion == "" || rep.UnsupportedNodeCount < 0 || rep.UnavailableNodeCount < 0 || rep.UnsupportedNodeCount+rep.UnavailableNodeCount >= rep.NodeCount {
		return "", fmt.Errorf("invalid parsed representation preimage")
	}
	if rep.State != "ready" && rep.State != "empty" && rep.State != "partial" {
		return "", fmt.Errorf("invalid parsed representation state")
	}
	return KnowledgeLifecycleDigest(map[string]any{"kind": "parsed-representation/0.2.0", "source_ref": rep.Source, "producer_sha256": *rep.Ref.ProducerSHA256, "reading_text_sha256": rep.ReadingTextSHA256, "reading_byte_length": rep.ReadingByteLength, "manifest_sha256": rep.ManifestSHA256, "assembly": rep.Assembly, "range_plan_version": rep.RangePlanVersion, "max_unit_bytes": rep.MaxUnitBytes, "node_count": rep.NodeCount, "state": rep.State, "unsupported_node_count": rep.UnsupportedNodeCount, "unavailable_node_count": rep.UnavailableNodeCount})
}

func KnowledgeParsedNodeDigest(representation string, index int64, payload string) (string, error) {
	if !knowledgeDocumentHex(representation, 64) || !knowledgeDocumentHex(payload, 64) || index < 0 || index >= 32768 {
		return "", fmt.Errorf("invalid parsed node preimage")
	}
	return KnowledgeLifecycleDigest(map[string]any{"kind": "parsed-node/0.2.0", "representation_id": representation, "local_index": index, "payload_sha256": payload})
}
