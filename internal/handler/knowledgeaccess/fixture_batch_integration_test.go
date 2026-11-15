//go:build integration && linux

package knowledgeaccess

import (
	"encoding/json"
	"time"

	"ragflow/internal/entity"
)

// The operator only binds/places the exact synthetic root. Project, observation,
// batch, manifest and structure task still go through their actual HTTP owners.
func (c *fixtureHTTP) prepareR1(domain, owner string) (entity.KnowledgeContentTarget, error) {
	var none entity.KnowledgeContentTarget
	ack, err := c.operatorStage("r1_root", domain, owner, "")
	if err != nil {
		return none, err
	}
	reply, err := c.exchange("directoryRoots", "owner", []string{domain}, nil, "")
	if err != nil {
		return none, err
	}
	var roots entity.KnowledgeDirectoryRoots
	if json.Unmarshal(reply.data, &roots) != nil || roots.DomainID != domain {
		return none, c.refuse()
	}
	ready := false
	for _, root := range roots.Items {
		if root.RootID == ack.RootID && root.Ready && root.RootVersion > 0 {
			ready = true
		}
	}
	if !ready {
		return none, c.refuse()
	}
	reply, err = fixtureJSON(c, "project", "owner", []string{domain}, entity.CreateKnowledgeGroupRequest{Kind: "project",
		Name: "Synthetic R1 " + c.scope.RunID, ComponentSide: "non_code"})
	if err != nil {
		return none, err
	}
	var project entity.KnowledgeGroup
	if json.Unmarshal(reply.data, &project) != nil || !isHex(project.ID, 32) || project.Kind != "project" {
		return none, c.refuse()
	}
	classification := entity.KnowledgeDirectoryClassification{GroupID: &project.ID, ComponentSide: "non_code"}
	reply, err = fixtureJSON(c, "directoryRegister", "owner", []string{domain}, entity.KnowledgeDirectoryRegistrationRequest{
		RegistrationKey: fixtureKey(c, "r1-directory"), Name: "Synthetic R1", SourceKind: "controlled_directory", InputMode: "server_read",
		RootID: ack.RootID, ScopeDirectories: []string{"."}, DefaultClassification: classification})
	if err != nil {
		return none, err
	}
	var reg entity.KnowledgeDirectoryRegistration
	if json.Unmarshal(reply.data, &reg) != nil || reg.RootID != ack.RootID || reg.DomainID != domain || !isHex(reg.RegistrationID, 32) {
		return none, c.refuse()
	}
	reply, err = fixtureJSON(c, "directoryObserve", "owner", []string{domain, reg.RegistrationID},
		entity.KnowledgeDirectoryObservationRequest{SelectedPaths: []string{"guide.md"}})
	if err != nil {
		return none, err
	}
	var observation entity.KnowledgeDirectoryObservation
	raw, err := fixtureMaterial("guide.md")
	if err != nil || json.Unmarshal(reply.data, &observation) != nil || !observation.Complete || len(observation.Items) != 1 ||
		!observation.Items[0].Eligible || observation.Items[0].SHA256 == nil || *observation.Items[0].SHA256 != digest(raw) {
		return none, c.refuse()
	}
	reply, err = fixtureJSON(c, "batchPreview", "owner", []string{domain, reg.RegistrationID}, entity.KnowledgeBatchPreviewRequest{
		ObservationID: observation.ObservationID, ObservationSHA256: observation.ObservationSHA256,
		SelectedItems: []entity.KnowledgeBatchSelectionItem{{RelativePath: "guide.md", Classification: classification}}})
	if err != nil {
		return none, err
	}
	var preview entity.KnowledgeBatchPreview
	if json.Unmarshal(reply.data, &preview) != nil || len(preview.Items) != 1 || preview.Items[0].SHA256 != digest(raw) ||
		preview.Items[0].RelativePath != "guide.md" || preview.Items[0].Action != "add" {
		return none, c.refuse()
	}
	reply, err = fixtureJSON(c, "batchAccept", "owner", []string{domain, reg.RegistrationID}, entity.KnowledgeBatchAcceptRequest{
		PreviewID: preview.PreviewID, ManifestSHA256: preview.ManifestSHA256, IdempotencyKey: fixtureKey(c, "r1-batch"),
		ConfirmedItemIDs: []string{preview.Items[0].ItemID}})
	if err != nil {
		return none, err
	}
	var task entity.KnowledgeBatchTask
	if json.Unmarshal(reply.data, &task) != nil || !isHex(task.TaskID, 32) {
		return none, c.refuse()
	}
	id := task.TaskID
	for poll := 0; poll < 6; poll++ {
		reply, err = c.exchange("batchGet", "owner", []string{domain, reg.RegistrationID, id}, nil, "")
		if err != nil {
			return none, err
		}
		if json.Unmarshal(reply.data, &task) != nil || task.TaskID != id || task.DomainID != domain {
			return none, c.refuse()
		}
		if task.Status == "succeeded" {
			return c.prepareR1FromBatch(domain, reg.RegistrationID, project.ID, id)
		}
		if task.Status != "queued" && task.Status != "running" {
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
