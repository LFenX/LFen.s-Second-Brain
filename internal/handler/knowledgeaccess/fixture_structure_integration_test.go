//go:build integration && linux

package knowledgeaccess

import (
	"encoding/json"
	"time"

	"ragflow/internal/entity"
)

// R1 continues a real directory batch. The batch is not replaced by handwritten
// source/view/manifest records. Root must separately provision the bound public
// synthetic directory root and its exact guide.md before this stage.
func (c *fixtureHTTP) prepareR1FromBatch(domain, registration, project, batch string) (entity.KnowledgeContentTarget, error) {
	var none entity.KnowledgeContentTarget
	for _, id := range []string{domain, registration, project, batch} {
		if !isHex(id, 32) {
			return none, c.refuse()
		}
	}
	reply, err := c.exchange("batchGet", "owner", []string{domain, registration, batch}, nil, "")
	if err != nil {
		return none, err
	}
	var completed entity.KnowledgeBatchTask
	if json.Unmarshal(reply.data, &completed) != nil || completed.TaskID != batch || completed.DomainID != domain ||
		completed.RegistrationID != registration || completed.Status != "succeeded" || completed.Counts.Total != 1 ||
		completed.Counts.Succeeded != 1 || completed.Counts.Failed != 0 {
		return none, c.refuse()
	}
	reply, err = fixtureJSON(c, "manifestPreview", "owner", []string{domain}, entity.KnowledgeProjectManifestPreviewRequest{ProjectID: project, TaskID: batch})
	if err != nil {
		return none, err
	}
	var preview entity.KnowledgeProjectManifestPreviewPage
	if json.Unmarshal(reply.data, &preview) != nil || preview.ProjectID != project || preview.DomainID != domain ||
		preview.TaskID != batch || len(preview.Items) != 1 || preview.NextCursor != nil {
		return none, c.refuse()
	}
	member := preview.Items[0]
	raw, err := fixtureMaterial("guide.md")
	if err != nil || member.RelativePath != "guide.md" || member.SHA256 != digest(raw) || member.ByteLength != int64(len(raw)) ||
		member.Classification.GroupID == nil || *member.Classification.GroupID != project {
		return none, c.refuse()
	}
	reply, err = fixtureJSON(c, "manifestPublish", "owner", []string{domain}, entity.KnowledgeProjectManifestPublicationRequest{
		ProjectID: project, PreviewID: preview.PreviewID, ManifestSHA256: preview.ManifestSHA256, IdempotencyKey: fixtureKey(c, "r1-manifest")})
	if err != nil {
		return none, err
	}
	var publication entity.KnowledgeProjectManifestReceipt
	if json.Unmarshal(reply.data, &publication) != nil || publication.ProjectID != project || publication.ManifestSHA256 != preview.ManifestSHA256 ||
		!isHex(publication.ManifestID, 32) {
		return none, c.refuse()
	}
	reply, err = c.exchange("lifecycle", "owner", []string{domain, member.SourceID}, nil, "")
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
	reply, err = c.exchange("structurePolicy", "owner", []string{domain}, nil, "")
	if err != nil {
		return none, err
	}
	var policy entity.KnowledgeStructurePolicy
	if json.Unmarshal(reply.data, &policy) != nil || policy.DomainID != domain || policy.FoundationPolicyVersion != 1 ||
		policy.ExternalModels || policy.LocalInference || policy.AutomaticIngestion || len(policy.AllowedModelUses) != 0 {
		return none, c.refuse()
	}
	if !policy.Enabled {
		reply, err = fixtureJSON(c, "structurePolicySet", "owner", []string{domain}, entity.KnowledgeStructurePolicyChangeRequest{
			ExpectedPolicyVersion: 1, ExpectedStructurePolicyVersion: policy.StructurePolicyVersion, Enabled: true,
			Confirmed: true, IdempotencyKey: fixtureKey(c, "r1-policy")})
		if err != nil {
			return none, err
		}
		var change entity.KnowledgeStructurePolicyReceipt
		if json.Unmarshal(reply.data, &change) != nil || change.DomainID != domain || !change.After.Enabled || change.ModelCalls != 0 {
			return none, c.refuse()
		}
		policy.StructurePolicyVersion = change.After.StructurePolicyVersion
	}
	req := entity.KnowledgeStructureTaskRequest{ProjectID: project, ManifestID: publication.ManifestID,
		ExpectedPolicyVersion: 1, ExpectedStructurePolicyVersion: policy.StructurePolicyVersion,
		Members: []entity.KnowledgeStructureMember{{SourceID: member.SourceID, SourceRevisionID: member.SourceRevisionID,
			LifecycleVersion: lifecycle.Current.Version, Parser: "markdown"}},
		Aliases: []entity.KnowledgeStructureAlias{}, Confirmed: true, IdempotencyKey: fixtureKey(c, "r1-task")}
	reply, err = fixtureJSON(c, "structureTask", "owner", []string{domain}, req)
	if err != nil {
		return none, err
	}
	var task entity.KnowledgeStructureTask
	if json.Unmarshal(reply.data, &task) != nil || !isHex(task.TaskID, 32) || !isHex(task.ProducerSHA256, 64) {
		return none, c.refuse()
	}
	id, producer := task.TaskID, task.ProducerSHA256
	for poll := 0; poll < 6; poll++ {
		reply, err = c.exchange("structureTaskGet", "owner", []string{domain, id}, nil, "")
		if err != nil {
			return none, err
		}
		if json.Unmarshal(reply.data, &task) != nil || task.TaskID != id || task.ProjectID != project || task.ProducerSHA256 != producer ||
			task.ModelCalls != 0 || task.LocalInference != 0 || task.ExternalFetches != 0 {
			return none, c.refuse()
		}
		if task.State == "succeeded" {
			if task.ResultViewID == nil || !isHex(*task.ResultViewID, 64) {
				return none, c.refuse()
			}
			view := *task.ResultViewID
			return entity.KnowledgeContentTarget{Source: entity.KnowledgeContentSourceRef{DomainID: domain,
				SourceID: member.SourceID, RevisionID: member.SourceRevisionID, SHA256: member.SHA256},
				Representation: entity.KnowledgeContentRepresentationRef{Kind: "structure_view", ID: &view, ProducerSHA256: &producer}}, nil
		}
		if task.State != "queued" && task.State != "running" {
			return none, c.refuse()
		}
		if time.Until(c.scope.Expires) < 3*time.Second {
			return none, c.refuse()
		}
		if c.pause(3*time.Second) != nil {
			return none, c.refuse()
		}
	}
	return none, c.refuse()
}
