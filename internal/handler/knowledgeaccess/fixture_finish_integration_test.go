//go:build integration && linux

package knowledgeaccess

import (
	"encoding/json"
	"reflect"
	"time"

	"ragflow/internal/entity"
)

func (c *fixtureHTTP) enableFixtureMaster(domain string) error {
	reply, err := c.exchange("localPolicy", "owner", []string{domain}, nil, "")
	if err != nil {
		return err
	}
	var policy entity.KnowledgeLexicalPolicy
	if json.Unmarshal(reply.data, &policy) != nil || policy.DomainID != domain || policy.FoundationVersion != 1 || policy.Enabled || policy.Version != 0 ||
		policy.ExternalModels || policy.LocalInference || policy.RemoteFetches || policy.AutomaticIngestion || len(policy.AllowedModelUses) != 0 {
		return c.refuse()
	}
	reply, err = fixtureJSON(c, "localPolicySet", "owner", []string{domain}, entity.KnowledgeLexicalPolicyChange{Version: entity.KnowledgeLocalLexicalVersion,
		Key: fixtureKey(c, "master-policy"), ExpectedVersion: policy.Version, ExpectedFoundationVersion: policy.FoundationVersion, Enabled: true})
	if err != nil {
		return err
	}
	var changed entity.KnowledgeLexicalPolicyReceipt
	if json.Unmarshal(reply.data, &changed) != nil || changed.Policy.DomainID != domain || !changed.Policy.Enabled || changed.Policy.Version != 1 ||
		changed.Policy.FoundationVersion != 1 || changed.Policy.ExternalModels || changed.Policy.LocalInference || changed.Policy.RemoteFetches ||
		changed.Policy.AutomaticIngestion || len(changed.Policy.AllowedModelUses) != 0 {
		return c.refuse()
	}
	return nil
}

func (c *fixtureHTTP) finishFixture(actors fixtureActors, domains map[string]string, selection entity.KnowledgeContentLexicalSelection,
	ar2 entity.KnowledgeContentSourceRef, names map[string]string, contracts *contractSet) (*setupFixture, error) {
	domain := domains["owner"]
	if c.enableFixtureMaster(domain) != nil {
		return nil, c.refuse()
	}
	job, err := c.publishOwnerFixture(domain, selection)
	if err != nil {
		return nil, err
	}
	// A fresh owner inspect verifies the published policy/profile and historical
	// A-r1 binding before any canonical operator capture or oracle construction.
	reply, err := fixtureJSON(c, "ownerInspect", "owner", []string{domain}, entity.KnowledgeContentLexicalInspectRequest{Version: entity.KnowledgeContentVersion, Selection: selection})
	if err != nil {
		return nil, err
	}
	var state entity.KnowledgeContentLexicalInspectData
	if json.Unmarshal(reply.data, &state) != nil || state.Readiness.Index == nil || job.Index == nil || !reflect.DeepEqual(*state.Readiness.Index, *job.Index) ||
		state.Readiness.Kind != "ready" || state.Readiness.Currentness != "mixed" || !state.Policy.Enabled || state.Profile.ModelsEnabled || state.Profile.RemoteFetches ||
		state.Profile.SHA256 != job.ProfileSHA256 || state.Profile.ProducerSHA256 != job.ProducerSHA256 || state.Policy.LocalVersion != job.LocalVersion ||
		state.Policy.ContentVersion != job.ContentVersion || state.Policy.FoundationVersion != job.FoundationVersion {
		return nil, c.refuse()
	}
	proof, err := c.collectOwnerProof(domain, actors, job, ar2, names)
	if err != nil {
		return nil, err
	}
	fixture, err := fixturePublicOracle(proof, actors, domains, contracts)
	if err != nil || fixture.Prepared || c.stopped || !time.Now().Before(c.scope.Expires) {
		return nil, c.refuse()
	}
	return fixture, nil
}

// Public facts and signed sessions use separate bounded RAM messages. Neither
// message grants S1 execution; the fixture remains prepared:false for Root review.
func (c *fixtureHTTP) handoffFixture(fixture *setupFixture, actors fixtureActors) error {
	if fixture == nil || fixture.Prepared || !c.scope.Handoff || c.stopped || !time.Now().Before(c.scope.Expires) {
		return c.refuse()
	}
	wire := fixtureJSONBytes(struct {
		Kind     string        `json:"kind"`
		RunID    string        `json:"run_id"`
		Fixture  *setupFixture `json:"fixture"`
		Attempts int           `json:"http_attempts"`
		Bytes    int           `json:"http_and_operator_json_bytes"`
	}{
		"fixture_candidate", c.scope.RunID, fixture, c.attempts, c.bodyBytes})
	if len(wire) > 131072 || c.bodyBytes+len(wire) > fixtureJSONBudget {
		return c.refuse()
	}
	if c.writeRAM(wire) != nil {
		return c.refuse()
	}
	return c.handoffSessions(actors.Public)
}
