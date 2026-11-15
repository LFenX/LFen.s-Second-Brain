//go:build integration && linux

package knowledgeaccess

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"strings"
	"time"

	"ragflow/internal/entity"
)

type fixtureActors struct {
	Public map[string]actorFixture
	emails map[string]string
}

// Passwords are freshly generated, used once in RAM and never returned.
// The native handler accepts plaintext; the "Fx!" prefix makes it unambiguously non-base64.
func (c *fixtureHTTP) prepareAccounts() (fixtureActors, error) {
	out := fixtureActors{Public: map[string]actorFixture{}, emails: map[string]string{}}
	for _, actor := range []string{"owner", "recipient", "foreign"} {
		secret, err := fixtureRandomID(24)
		if err != nil {
			return out, c.refuse()
		}
		email := "fixture-" + c.scope.RunID + "-" + actor + "@fixtures.invalid"
		reply, err := fixtureJSON(c, "register", "anonymous", nil, map[string]any{
			"email": email, "password": "Fx!" + secret, "nickname": "Synthetic " + actor + " " + c.scope.RunID})
		secret = ""
		if err != nil {
			return out, err
		}
		var profile struct {
			ID string "json:\"id\""
		}
		if json.Unmarshal(reply.data, &profile) != nil || !isHex(profile.ID, 32) {
			return out, c.refuse()
		}
		for _, known := range out.Public {
			if known.UserID == profile.ID {
				return out, c.refuse()
			}
		}
		for _, known := range c.sessions {
			if known == reply.authorization {
				return out, c.refuse()
			}
		}
		c.sessions[actor] = reply.authorization
		// No document, source, domain or task is created before this tenant's check.
		models, err := c.exchange("models", actor, nil, nil, "")
		if err != nil {
			return out, err
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(models.data, &fields) != nil {
			return out, c.refuse()
		}
		for _, key := range []string{"llm_id", "embd_id", "rerank_id", "asr_id", "img2txt_id", "tts_id"} {
			raw, exists := fields[key]
			var value string
			if !exists || json.Unmarshal(raw, &value) != nil || value != "" {
				return out, c.refuse()
			}
		}
		var tenant, role string
		if json.Unmarshal(fields["tenant_id"], &tenant) != nil || tenant != profile.ID ||
			json.Unmarshal(fields["role"], &role) != nil || role != "owner" {
			return out, c.refuse()
		}
		out.Public[actor] = actorFixture{UserID: profile.ID, TenantID: tenant}
		out.emails[actor] = email
	}
	// HTTP handler AddTenantMember checks tenantSMTPConfig before sending.
	// Root attests that exact guard false before registration/invite.
	if c.scope.Safety.SMTPEnabled || !c.scope.Safety.SMTPDisabledProven {
		return out, c.refuse()
	}
	owner := out.Public["owner"]
	invited, err := fixtureJSON(c, "invite", "owner", []string{owner.TenantID}, map[string]any{"email": out.emails["recipient"]})
	if err != nil {
		return out, err
	}
	var invite struct {
		ID string "json:\"id\""
	}
	if json.Unmarshal(invited.data, &invite) != nil || invite.ID != out.Public["recipient"].UserID {
		return out, c.refuse()
	}
	accepted, err := c.exchange("accept", "recipient", []string{owner.TenantID}, nil, "")
	if err != nil || string(accepted.data) != "true" {
		return out, c.refuse()
	}
	members, err := c.exchange("members", "owner", []string{owner.TenantID}, nil, "")
	if err != nil {
		return out, err
	}
	var rows []struct {
		UserID string "json:\"user_id\""
		Role   string "json:\"role\""
	}
	if json.Unmarshal(members.data, &rows) != nil {
		return out, c.refuse()
	}
	found := false
	for _, row := range rows {
		if row.UserID == invite.ID && row.Role == "normal" {
			found = true
		}
	}
	if !found {
		return out, c.refuse()
	}
	recipient := out.Public["recipient"]
	recipient.TenantID = owner.TenantID
	out.Public["recipient"] = recipient
	return out, nil
}

func (c *fixtureHTTP) prepareDomains(actors fixtureActors) (map[string]string, error) {
	domains := map[string]string{}
	for _, actor := range []string{"owner", "foreign"} {
		reply, err := fixtureJSON(c, "dataset", actor, nil, map[string]any{
			"name":       "Synthetic fixture " + c.scope.RunID + " " + actor,
			"parse_type": 1, "parser_id": "naive", "permission": "me", "parser_config": map[string]any{}})
		if err != nil {
			return domains, err
		}
		var dataset struct {
			ID string "json:\"id\""
		}
		if json.Unmarshal(reply.data, &dataset) != nil || !isHex(dataset.ID, 32) {
			return domains, c.refuse()
		}
		registered, err := fixtureJSON(c, "domain", actor, []string{dataset.ID}, entity.RegisterKnowledgeDomainRequest{Category: "custom"})
		if err != nil {
			return domains, err
		}
		var domain entity.KnowledgeDomain
		if json.Unmarshal(registered.data, &domain) != nil || domain.ID != dataset.ID ||
			domain.OwnerID != actors.Public[actor].UserID || domain.ContractVersion != "1.0.0" ||
			domain.Policy.Version != 1 || domain.Policy.Access != "owner_only" ||
			domain.Policy.ExternalModels || domain.Policy.AutomaticIngestion ||
			len(domain.Policy.AllowedModelUses) != 0 || len(domain.Policy.IndexModes) != 1 ||
			domain.Policy.IndexModes[0] != "catalogue" {
			return domains, c.refuse()
		}
		domains[actor] = dataset.ID
	}
	if domains["owner"] == domains["foreign"] {
		return domains, c.refuse()
	}
	return domains, nil
}

func fixtureMultipart(name string, raw []byte, request any) ([]byte, string, error) {
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	if request != nil {
		body, err := fixtureEncode(request)
		if err != nil {
			return nil, "", err
		}
		if writer.WriteField("request", string(body)) != nil {
			return nil, "", errFixture
		}
	}
	part, err := writer.CreateFormFile("file", name)
	if err != nil {
		return nil, "", errFixture
	}
	if _, err = part.Write(raw); err != nil {
		return nil, "", errFixture
	}
	if writer.Close() != nil || buffer.Len() > 32768 {
		return nil, "", errFixture
	}
	return buffer.Bytes(), writer.FormDataContentType(), nil
}

func fixtureKey(c *fixtureHTTP, label string) string {
	return digest([]byte(c.scope.RunID + ":" + label))
}

func (c *fixtureHTTP) parsedPolicy(domain string) (entity.KnowledgeContentPolicy, error) {
	reply, err := c.exchange("parsedPolicy", "owner", []string{domain}, nil, "")
	var policy entity.KnowledgeContentPolicy
	if err != nil {
		return policy, err
	}
	if json.Unmarshal(reply.data, &policy) != nil || policy.DomainID != domain || policy.FoundationPolicyVersion != 1 {
		return policy, c.refuse()
	}
	if !policy.ParseEnabled || !policy.LexicalEnabled {
		reply, err = fixtureJSON(c, "parsedPolicySet", "owner", []string{domain}, map[string]any{
			"contract_version": entity.KnowledgeContentVersion, "request": entity.KnowledgeContentPolicyRequest{
				ExpectedPolicyVersion: policy.FoundationPolicyVersion, ExpectedContentPolicyVersion: policy.ContentPolicyVersion,
				ParseEnabled: true, LexicalEnabled: true, Confirmed: true, IdempotencyKey: fixtureKey(c, "content-policy")}})
		if err != nil {
			return policy, err
		}
		if json.Unmarshal(reply.data, &policy) != nil || policy.DomainID != domain || !policy.ParseEnabled || !policy.LexicalEnabled {
			return policy, c.refuse()
		}
	}
	return policy, nil
}

// Upload/snapshot/producer-bound task/poll all use the existing parsed owner API.
// Re-upload is intentionally not used to pretend that A-r2 is a revision of A.
func (c *fixtureHTTP) prepareParsed(domain, name string) (entity.KnowledgeContentTarget, error) {
	var none entity.KnowledgeContentTarget
	raw, err := fixtureMaterial(name)
	if err != nil || !strings.HasSuffix(name, ".csv") {
		return none, c.refuse()
	}
	policy, err := c.parsedPolicy(domain)
	if err != nil {
		return none, err
	}
	body, media, err := fixtureMultipart(name, raw, map[string]any{"contract_version": entity.KnowledgeContentVersion, "request": entity.KnowledgeParsedUploadRequest{ExpectedPolicyVersion: 1, ExpectedContentPolicyVersion: policy.ContentPolicyVersion, Confirmed: true}})
	if err != nil {
		return none, c.refuse()
	}
	reply, err := c.exchange("parsedUpload", "owner", []string{domain}, body, media)
	if err != nil {
		return none, err
	}
	var uploaded entity.KnowledgeParsedUploadReceipt
	if json.Unmarshal(reply.data, &uploaded) != nil || !isHex(uploaded.SourceID, 32) || uploaded.ByteLength != int64(len(raw)) {
		return none, c.refuse()
	}
	_, err = fixtureJSON(c, "parsedSnapshot", "owner", []string{domain}, map[string]any{"contract_version": entity.KnowledgeContentVersion, "request": entity.KnowledgeParsedSnapshotRequest{DocumentID: uploaded.SourceID, ExpectedRevisionID: "",
		ExpectedPolicyVersion: 1, RelativePath: name, Classification: entity.KnowledgeDirectoryClassification{ComponentSide: "non_code"}}})
	if err != nil {
		return none, err
	}
	reply, err = fixtureJSON(c, "parsedSource", "owner", []string{domain}, map[string]any{"contract_version": entity.KnowledgeContentVersion, "source_id": uploaded.SourceID})
	if err != nil {
		return none, err
	}
	var state entity.KnowledgeParsedSourceState
	if json.Unmarshal(reply.data, &state) != nil || state.Source.DomainID != domain || state.Source.SourceID != uploaded.SourceID ||
		state.Source.SHA256 != digest(raw) || state.Profile == nil || !state.Profile.RuntimeReady ||
		!isHex(state.Profile.ProducerSHA256, 64) || state.LifecycleVersion < 1 || !isHex(state.NativeFileID, 32) || state.ByteLength != int64(len(raw)) {
		return none, c.refuse()
	}
	c.parsedBindings[state.Source.SourceID] = fixtureParsedBinding{Source: state.Source, NativeFileID: state.NativeFileID, Bytes: state.ByteLength}
	req := entity.KnowledgeParsedTaskRequest{Source: state.Source, ExpectedLifecycleVersion: state.LifecycleVersion,
		ExpectedPolicyVersion: 1, ExpectedContentPolicyVersion: policy.ContentPolicyVersion,
		ExpectedProducerSHA256: state.Profile.ProducerSHA256, MaxUnitBytes: 8192, Confirmed: true, IdempotencyKey: fixtureKey(c, name)}
	reply, err = fixtureJSON(c, "parsedTask", "owner", []string{domain}, map[string]any{"contract_version": entity.KnowledgeContentVersion, "request": req})
	if err != nil {
		return none, err
	}
	var task entity.KnowledgeParsedTask
	if json.Unmarshal(reply.data, &task) != nil || !isHex(task.TaskID, 32) || task.OriginalRequest.Source != req.Source {
		return none, c.refuse()
	}
	for poll := 0; poll < 6; poll++ {
		reply, err = fixtureJSON(c, "parsedTaskGet", "owner", []string{domain}, map[string]any{"contract_version": entity.KnowledgeContentVersion, "task_id": task.TaskID})
		if err != nil {
			return none, err
		}
		if json.Unmarshal(reply.data, &task) != nil || task.DomainID != domain || task.OriginalRequest.Source != req.Source {
			return none, c.refuse()
		}
		if task.State == "succeeded" {
			if task.Result == nil || task.Result.Source != req.Source || task.Result.Representation.Kind != "parsed_representation" ||
				task.Result.Representation.ProducerSHA256 == nil || *task.Result.Representation.ProducerSHA256 != req.ExpectedProducerSHA256 {
				return none, c.refuse()
			}
			return *task.Result, nil
		}
		if task.State != "queued" && task.State != "running" {
			return none, c.refuse()
		}
		if c.pause(2*time.Second) != nil {
			return none, c.refuse()
		}
	}
	return none, c.refuse()
}

// A source snapshot descriptor is never synthesized from a filename or sample ID.
func (c *fixtureHTTP) prepareC1(domain string) (entity.KnowledgeContentSourceRef, error) {
	var none entity.KnowledgeContentSourceRef
	raw, err := fixtureMaterial("long-source.txt")
	if err != nil {
		return none, c.refuse()
	}
	body, media, err := fixtureMultipart("long-source.txt", raw, nil)
	if err != nil {
		return none, c.refuse()
	}
	reply, err := c.exchange("nativeUpload", "owner", []string{domain}, body, media)
	if err != nil {
		return none, err
	}
	var docs []struct {
		ID string "json:\"id\""
	}
	if json.Unmarshal(reply.data, &docs) != nil || len(docs) != 1 || !isHex(docs[0].ID, 32) {
		return none, c.refuse()
	}
	reply, err = fixtureJSON(c, "sourceSnapshot", "owner", []string{domain}, entity.SnapshotKnowledgeSourceRequest{
		DocumentID: docs[0].ID, ExpectedRevisionID: "", RelativePath: "long-source.txt", ComponentSide: "non_code"})
	if err != nil {
		return none, err
	}
	var result struct {
		Source entity.KnowledgeSourceRevision "json:\"source\""
	}
	if json.Unmarshal(reply.data, &result) != nil || result.Source.DomainID != domain || result.Source.DocumentID != docs[0].ID ||
		result.Source.SHA256 != digest(raw) || result.Source.ByteLength != int64(len(raw)) {
		return none, c.refuse()
	}
	return entity.KnowledgeContentSourceRef{DomainID: domain, SourceID: result.Source.DocumentID, RevisionID: result.Source.ID, SHA256: result.Source.SHA256}, nil
}
