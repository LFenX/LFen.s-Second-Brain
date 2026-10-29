package dataset

import (
	"gorm.io/gorm"
	"ragflow/internal/entity"
)

type contentLexicalSourceFence struct {
	SourceID string `json:"source_id"`
	SHA256   string `json:"fence_sha256"`
}

type contentLexicalInputs struct {
	Version         string                      `json:"version"`
	SelectionSHA256 string                      `json:"selection_sha256"`
	ProfileSHA256   string                      `json:"profile_sha256"`
	ProducerSHA256  string                      `json:"producer_sha256"`
	Policies        contentLexicalPolicyCapture `json:"policies"`
	Sources         []contentLexicalSourceFence `json:"sources"`
}

// The common source/lifecycle owner supports all four representation families.
// Fences are captured under admission/publication locks; their private native
// metadata never enters an HTTP or MCP response.
func contentLexicalCaptureSources(tx *gorm.DB, kb *entity.Knowledgebase, selection entity.KnowledgeContentLexicalSelection) ([]contentLexicalSourceFence, error) {
	tx = tx.Session(&gorm.Session{})
	out := make([]contentLexicalSourceFence, 0, len(selection.Targets))
	for _, target := range selection.Targets {
		data, err := loadLifecycleSource(tx, kb, target.Source.SourceID)
		if err != nil {
			return nil, contentLexicalNormalize(err)
		}
		var fixed entity.KnowledgeSourceRevision
		if err := tx.Where("id = ? AND domain_id = ? AND document_id = ?", target.Source.RevisionID, kb.ID, target.Source.SourceID).Take(&fixed).Error; err != nil {
			return nil, contentLexicalNormalize(err)
		}
		if fixed.SHA256 != target.Source.SHA256 {
			return nil, lexicalError(409, "integrity_error")
		}
		shape := map[string]any{"kind": "content-lexical-source-fence/0.2.0", "target": target, "head_revision_id": data.head.ID, "head_input_sha256": data.head.InputHash, "lifecycle_version": data.state.Version, "lifecycle_state": data.state.State, "fixed_input_sha256": fixed.InputHash, "fixed_kind": fixed.RepresentationKind, "fixed_file_id": fixed.FileID, "fixed_commit_id": fixed.CommitID, "fixed_folder_id": fixed.FolderID, "fixed_byte_length": fixed.ByteLength, "native_file_id": data.native.ID, "native_parent_id": data.native.ParentID, "native_tenant_id": data.native.TenantID, "native_name": data.native.Name, "native_location": data.native.Location, "native_size": data.native.Size, "native_source_type": data.native.SourceType}
		switch target.Representation.Kind {
		case "document_representation", "parsed_representation":
			protocol := documentTaskProtocol
			if target.Representation.Kind == "parsed_representation" {
				protocol = entity.KnowledgeParsedProtocol
			}
			var row entity.KnowledgeDocumentRepresentationRecord
			if err := tx.Where("id = ? AND owner_id = ? AND domain_id = ? AND source_id = ? AND source_revision_id = ? AND protocol = ?", *target.Representation.ID, kb.CreatedBy, kb.ID, fixed.DocumentID, fixed.ID, protocol).Take(&row).Error; err != nil {
				return nil, contentLexicalNormalize(err)
			}
			if len(row.ReadingText) > 4<<20 || len(row.Manifest) > 32<<20 || len(row.Payload) > 32<<20 {
				return nil, lexicalError(409, "integrity_error")
			}
			shape["representation_payload_sha256"], shape["representation_manifest_sha256"], shape["reading_text_sha256"] = lexicalSHA([]byte(row.Payload)), lexicalSHA([]byte(row.Manifest)), lexicalSHA([]byte(row.ReadingText))
		case "structure_view":
			var row entity.KnowledgeStructureViewRecord
			if err := tx.Where("id = ? AND owner_id = ? AND domain_id = ?", *target.Representation.ID, kb.CreatedBy, kb.ID).Take(&row).Error; err != nil {
				return nil, contentLexicalNormalize(err)
			}
			shape["view_payload_sha256"] = lexicalSHA([]byte(row.Payload))
		}
		digest, err := entity.KnowledgeLifecycleDigest(shape)
		if err != nil {
			return nil, lexicalError(409, "integrity_error")
		}
		out = append(out, contentLexicalSourceFence{SourceID: target.Source.SourceID, SHA256: digest})
	}
	return out, nil
}

func contentLexicalValidateInputs(tx *gorm.DB, kb *entity.Knowledgebase, job entity.KnowledgeLexicalJobRecord, prepared *contentLexicalPrepared) error {
	tx = tx.Session(&gorm.Session{})
	var expected contentLexicalInputs
	if entity.KnowledgeParsedDecode([]byte(job.ContentInputs), &expected) != nil || expected.Version != entity.KnowledgeContentVersion || expected.SelectionSHA256 != prepared.SelectionSHA256 || expected.ProfileSHA256 != prepared.ProfileSHA256 || expected.ProducerSHA256 != prepared.ProducerSHA256 || expected.Policies != prepared.Policies {
		return lexicalError(409, "integrity_error")
	}
	policy, err := parsedPolicyRecord(tx, job.OwnerID, job.DomainID)
	if err != nil {
		return contentLexicalNormalize(err)
	}
	if !policy.LexicalEnabled {
		return lexicalError(409, "policy_disabled")
	}
	if policy.Version != job.ContentVersion || policy.Version != expected.Policies.ContentVersion {
		return lexicalError(409, "policy_changed")
	}
	current, err := contentLexicalCaptureSources(tx, kb, prepared.Selection)
	if err != nil {
		return err
	}
	if lexicalJSON(current) != lexicalJSON(expected.Sources) {
		return lexicalError(409, "source_changed")
	}
	return nil
}
