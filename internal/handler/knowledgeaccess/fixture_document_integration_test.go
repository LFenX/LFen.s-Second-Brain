//go:build integration && linux

package knowledgeaccess

import (
	"encoding/json"
	"time"

	"ragflow/internal/entity"
)

// C2 registration/upload/confirm plus the real native DOCX task.
// Lifecycle comes from the owner lifecycle route, never a test-invented generation.
func (c *fixtureHTTP) prepareDOCX(domain string) (entity.KnowledgeContentTarget, error) {
	var none entity.KnowledgeContentTarget
	raw, err := fixtureMaterial("empty-cell.docx")
	if err != nil {
		return none, c.refuse()
	}
	reply, err := fixtureJSON(c, "originalRegister", "owner", []string{domain}, entity.KnowledgeOriginalRegistrationRequest{
		RegistrationKey: fixtureKey(c, "docx-registration"), Name: "Synthetic DOCX " + c.scope.RunID,
		SourceKind: "original_document", InputMode: "original_upload", IncludePaths: []string{"empty-cell.docx"},
		DefaultClassification: entity.KnowledgeDirectoryClassification{ComponentSide: "non_code"}})
	if err != nil {
		return none, err
	}
	var reg entity.KnowledgeOriginalRegistration
	if json.Unmarshal(reply.data, &reg) != nil || reg.DomainID != domain || !isHex(reg.RegistrationID, 32) {
		return none, c.refuse()
	}
	hash := digest(raw)
	reply, err = fixtureJSON(c, "originalUpload", "owner", []string{domain, reg.RegistrationID}, entity.KnowledgeOriginalUploadRequest{
		OriginalFilename: "empty-cell.docx", RelativePath: "empty-cell.docx",
		Classification:     entity.KnowledgeDirectoryClassification{ComponentSide: "non_code"},
		DeclaredByteLength: int64(len(raw)), DeclaredMediaType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		ExpectedSHA256: &hash, IdempotencyKey: fixtureKey(c, "docx-upload")})
	if err != nil {
		return none, err
	}
	var upload entity.KnowledgeOriginalUpload
	if json.Unmarshal(reply.data, &upload) != nil || !isHex(upload.UploadID, 32) || upload.DomainID != domain {
		return none, c.refuse()
	}
	reply, err = c.exchange("originalBytes", "owner", []string{domain, reg.RegistrationID, upload.UploadID}, raw, "application/octet-stream")
	if err != nil {
		return none, err
	}
	if json.Unmarshal(reply.data, &upload) != nil || upload.State != "ready" || upload.ManifestSHA256 == nil || !isHex(*upload.ManifestSHA256, 64) {
		return none, c.refuse()
	}
	reply, err = fixtureJSON(c, "originalConfirm", "owner", []string{domain, reg.RegistrationID}, entity.KnowledgeOriginalConfirmRequest{
		UploadID: upload.UploadID, ManifestSHA256: *upload.ManifestSHA256, IdempotencyKey: fixtureKey(c, "docx-confirm"), Confirmed: true})
	if err != nil {
		return none, err
	}
	var receipt entity.KnowledgeOriginalReceipt
	if json.Unmarshal(reply.data, &receipt) != nil || receipt.DomainID != domain || receipt.Source.SHA256 != hash ||
		receipt.Source.ByteLength != int64(len(raw)) || receipt.ModelCalls != 0 || !isHex(receipt.Source.SourceID, 32) ||
		!isHex(receipt.Source.SourceRevisionID, 32) {
		return none, c.refuse()
	}
	reply, err = c.exchange("lifecycle", "owner", []string{domain, receipt.Source.SourceID}, nil, "")
	if err != nil {
		return none, err
	}
	var lifecycle struct {
		Current struct {
			Version int64 "json:\"lifecycle_version\""
		} "json:\"current\""
	}
	if json.Unmarshal(reply.data, &lifecycle) != nil || lifecycle.Current.Version < 1 {
		return none, c.refuse()
	}
	reply, err = c.exchange("documentCapabilities", "owner", nil, nil, "")
	if err != nil {
		return none, err
	}
	var capabilities entity.KnowledgeDocumentCapabilities
	if json.Unmarshal(reply.data, &capabilities) != nil || !capabilities.RuntimeReady ||
		capabilities.ModelCalls != 0 || capabilities.ExternalFetches != 0 || capabilities.LocalInference != 0 || capabilities.AutomaticProcessing {
		return none, c.refuse()
	}
	var profile *entity.KnowledgeDocumentProfile
	for i := range capabilities.Profiles {
		p := &capabilities.Profiles[i]
		if p.ProfileID == entity.KnowledgeDocumentDOCXProfile {
			profile = p
		}
	}
	if profile == nil || !profile.RuntimeReady || profile.Configuration.OCR || profile.Configuration.Models ||
		profile.Configuration.RemoteResources || !isHex(profile.ProducerSHA256, 64) {
		return none, c.refuse()
	}
	reply, err = c.exchange("documentPolicy", "owner", []string{domain}, nil, "")
	if err != nil {
		return none, err
	}
	var policy entity.KnowledgeDocumentPolicy
	if json.Unmarshal(reply.data, &policy) != nil || policy.DomainID != domain || policy.FoundationPolicyVersion != 1 ||
		policy.ExternalModels || policy.LocalInference || policy.AutomaticProcessing || len(policy.AllowedModelUses) != 0 {
		return none, c.refuse()
	}
	if !policy.Enabled {
		reply, err = fixtureJSON(c, "documentPolicySet", "owner", []string{domain}, entity.KnowledgeDocumentPolicyChangeRequest{
			ExpectedPolicyVersion: 1, ExpectedDocumentPolicyVersion: policy.DocumentPolicyVersion, Enabled: true,
			Confirmed: true, IdempotencyKey: fixtureKey(c, "docx-policy")})
		if err != nil {
			return none, err
		}
		var change entity.KnowledgeDocumentPolicyReceipt
		if json.Unmarshal(reply.data, &change) != nil || change.DomainID != domain || !change.After.Enabled || change.ModelCalls != 0 {
			return none, c.refuse()
		}
		policy.DocumentPolicyVersion = change.After.DocumentPolicyVersion
	}
	req := entity.KnowledgeDocumentTaskRequest{Target: entity.KnowledgeDocumentOriginalTarget{
		Kind: "original_document", SourceID: receipt.Source.SourceID, SourceRevisionID: receipt.Source.SourceRevisionID,
		OriginalSHA256: hash, ExpectedLifecycleVersion: lifecycle.Current.Version}, ExpectedPolicyVersion: 1,
		ExpectedDocumentPolicyVersion: policy.DocumentPolicyVersion, ProfileID: profile.ProfileID,
		ExpectedProducerSHA256: profile.ProducerSHA256, Confirmed: true, IdempotencyKey: fixtureKey(c, "docx-task")}
	reply, err = fixtureJSON(c, "documentTask", "owner", []string{domain}, req)
	if err != nil {
		return none, err
	}
	var task entity.KnowledgeDocumentTask
	if json.Unmarshal(reply.data, &task) != nil || !isHex(task.TaskID, 32) {
		return none, c.refuse()
	}
	for poll := 0; poll < 6; poll++ {
		reply, err = c.exchange("documentTaskGet", "owner", []string{domain, task.TaskID}, nil, "")
		if err != nil {
			return none, err
		}
		if json.Unmarshal(reply.data, &task) != nil || task.DomainID != domain || task.OriginalRequest.Target != req.Target ||
			task.ProducerSHA256 != profile.ProducerSHA256 || task.ModelCalls != 0 || task.ExternalFetches != 0 || task.LocalInference != 0 {
			return none, c.refuse()
		}
		if task.State == "succeeded" {
			if task.ResultRepresentationID == nil || !isHex(*task.ResultRepresentationID, 64) {
				return none, c.refuse()
			}
			id, producer := *task.ResultRepresentationID, task.ProducerSHA256
			return entity.KnowledgeContentTarget{Source: entity.KnowledgeContentSourceRef{
				DomainID: domain, SourceID: receipt.Source.SourceID, RevisionID: receipt.Source.SourceRevisionID, SHA256: hash},
				Representation: entity.KnowledgeContentRepresentationRef{Kind: "document_representation", ID: &id, ProducerSHA256: &producer}}, nil
		}
		if task.State != "queued" && task.State != "running" {
			return none, c.refuse()
		}
		if time.Until(c.scope.Expires) < 2*time.Second {
			return none, c.refuse()
		}
		if c.pause(2*time.Second) != nil {
			return none, c.refuse()
		}
	}
	return none, c.refuse()
}

// This is the sole final owner0.2 publication. Caller must supply actual four
// family targets plus B only after A-r2 and every source/policy fence is frozen.
func (c *fixtureHTTP) publishOwnerFixture(domain string, selection entity.KnowledgeContentLexicalSelection) (entity.KnowledgeContentLexicalJob, error) {
	var job entity.KnowledgeContentLexicalJob
	if selection.Kind != "fixed_representations" || len(selection.Targets) != 5 {
		return job, c.refuse()
	}
	reply, err := fixtureJSON(c, "ownerInspect", "owner", []string{domain}, entity.KnowledgeContentLexicalInspectRequest{
		Version: entity.KnowledgeContentVersion, Selection: selection})
	if err != nil {
		return job, err
	}
	var state entity.KnowledgeContentLexicalInspectData
	if json.Unmarshal(reply.data, &state) != nil || !state.Policy.Enabled || state.Profile.ModelsEnabled ||
		state.Profile.RemoteFetches || state.Profile.MaxUnitBytes != 8192 || state.Readiness.Kind != "unindexed" ||
		!isHex(state.Profile.SHA256, 64) || !isHex(state.Profile.ProducerSHA256, 64) {
		return job, c.refuse()
	}
	reply, err = fixtureJSON(c, "ownerIndex", "owner", []string{domain}, entity.KnowledgeContentLexicalCreateRequest{
		Version: entity.KnowledgeContentVersion, Key: fixtureKey(c, "final-publication"),
		ExpectedLocalVersion: state.Policy.LocalVersion, ExpectedContentVersion: state.Policy.ContentVersion,
		ExpectedFoundationVersion: state.Policy.FoundationVersion, ExpectedProfileSHA256: state.Profile.SHA256, Selection: selection})
	if err != nil {
		return job, err
	}
	if json.Unmarshal(reply.data, &job) != nil || !isHex(job.ID, 32) {
		return job, c.refuse()
	}
	id := job.ID
	for poll := 0; poll < 6; poll++ {
		reply, err = fixtureJSON(c, "ownerJob", "owner", []string{domain}, entity.KnowledgeContentLexicalJobRequest{Version: entity.KnowledgeContentVersion, ID: id})
		if err != nil {
			return job, err
		}
		if json.Unmarshal(reply.data, &job) != nil || job.ID != id || job.ProfileSHA256 != state.Profile.SHA256 {
			return job, c.refuse()
		}
		if job.State == "published" {
			if job.Index == nil {
				return job, c.refuse()
			}
			return job, nil
		}
		if job.State != "queued" && job.State != "building" {
			return job, c.refuse()
		}
		if time.Until(c.scope.Expires) < 2*time.Second {
			return job, c.refuse()
		}
		if c.pause(2*time.Second) != nil {
			return job, c.refuse()
		}
	}
	return job, c.refuse()
}
