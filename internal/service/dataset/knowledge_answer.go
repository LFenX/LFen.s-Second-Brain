package dataset

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

type KnowledgeAnswerError struct {
	Status int
	Code   string
	ID     *string
}

func (e *KnowledgeAnswerError) Error() string {
	return "Knowledge answer operation could not be completed"
}
func answerFailure(status int, code string) error {
	return &KnowledgeAnswerError{Status: status, Code: code}
}

func normalizeAnswerError(err error) *KnowledgeAnswerError {
	var own *KnowledgeAnswerError
	if errors.As(err, &own) {
		return own
	}
	var lexical *KnowledgeLexicalError
	if errors.As(err, &lexical) {
		switch lexical.Status {
		case 401:
			return &KnowledgeAnswerError{Status: 401, Code: "unauthenticated"}
		case 404:
			return &KnowledgeAnswerError{Status: 404, Code: "not_found"}
		case 400:
			return &KnowledgeAnswerError{Status: 400, Code: "invalid_request"}
		case 409, 412, 415:
			return &KnowledgeAnswerError{Status: 412, Code: "index_stale"}
		case 413:
			return &KnowledgeAnswerError{Status: 413, Code: "capacity_exceeded"}
		}
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &KnowledgeAnswerError{Status: 404, Code: "not_found"}
	}
	var knowledge *KnowledgeError
	if errors.As(err, &knowledge) && knowledge.Status == 404 {
		return &KnowledgeAnswerError{Status: 404, Code: "not_found"}
	}
	return &KnowledgeAnswerError{Status: 503, Code: "unavailable"}
}

type answerModel struct {
	ref               entity.KnowledgeAnswerModelRef
	name, destination string
	generate          func(context.Context, string, []entity.KnowledgeAnswerCitation) (string, *entity.KnowledgeAnswerUsage, error)
}

type answerRunner struct {
	dataset *DatasetService
	resolve func(context.Context, string) (*answerModel, error)
	// Production always uses the owning C3 services. Focused unit tests inject
	// controlled retrieval/provider boundaries without external services.
	checkScope func(context.Context, string, string, entity.KnowledgeAnswerScope) error
	search     func(context.Context, string, string, entity.KnowledgeContentLexicalSearchRequest) (*entity.KnowledgeContentLexicalSearchData, error)
	read       func(context.Context, string, string, entity.KnowledgeContentLexicalReadRequest) (*entity.KnowledgeContentLexicalReadData, error)
}

func newAnswerRunner(d *DatasetService) *answerRunner {
	r := &answerRunner{dataset: d, search: d.SearchKnowledgeContentLexical, read: d.ReadKnowledgeContentLexical}
	r.resolve = func(ctx context.Context, owner string) (*answerModel, error) {
		m, err := service.ResolveKnowledgeAnswerModel(ctx, owner)
		if err != nil {
			if errors.Is(err, service.ErrKnowledgeAnswerModelUnsupported) {
				return nil, answerFailure(412, "unsupported_model")
			}
			return nil, answerFailure(503, "unavailable")
		}
		return &answerModel{ref: m.Ref, name: m.Name, destination: m.Destination, generate: m.Generate}, nil
	}
	r.checkScope = func(ctx context.Context, owner, domain string, scope entity.KnowledgeAnswerScope) error {
		return r.scopeError(ctx, owner, domain, d.checkAnswerScope(ctx, owner, domain, scope))
	}
	return r
}

// Once domain ownership is established, a missing fixed source/publication is
// a changed scope. A lost domain permit keeps its original closed denial.
func (r *answerRunner) scopeError(ctx context.Context, owner, domain string, err error) error {
	if err == nil || normalizeAnswerError(err).Status != 404 {
		return err
	}
	if _, _, permitErr := r.dataset.lexicalContext(ctx, owner, domain, false); permitErr != nil {
		return permitErr
	}
	return answerFailure(412, "scope_changed")
}

func (d *DatasetService) checkAnswerScope(ctx context.Context, owner, domain string, scope entity.KnowledgeAnswerScope) error {
	if scope.Selection.Validate(domain) != nil || !entity.ValidKnowledgeLexicalHash(scope.Index.ID) || !entity.ValidKnowledgeLexicalHash(scope.Index.ManifestSHA256) {
		return answerFailure(400, "invalid_request")
	}
	_, manifest, err := d.contentLexicalPublished(ctx, owner, domain, scope.Index, true)
	if err != nil {
		return err
	}
	selection, sha := contentLexicalSelection(scope.Selection)
	if sha != manifest.SelectionSHA256 || lexicalJSON(selection) != lexicalJSON(manifest.Selection) {
		return answerFailure(412, "scope_changed")
	}
	_, err = d.contentLexicalCurrentness(ctx, owner, domain, selection)
	return err
}

func normalizeAnswerScope(scope entity.KnowledgeAnswerScope) entity.KnowledgeAnswerScope {
	scope.Selection, _ = contentLexicalSelection(scope.Selection)
	return scope
}

func answerPolicy(row entity.KnowledgeAnswerPolicyRecord) (entity.KnowledgeAnswerPolicy, error) {
	p := entity.KnowledgeAnswerPolicy{DomainID: row.DomainID, Version: row.Version, Enabled: row.Enabled}
	if row.Version < 0 || row.Version > entity.KnowledgeAccessMaxGeneration {
		return p, answerFailure(503, "unavailable")
	}
	if row.Version > 0 {
		actor, at := row.ConsentActor, accessTime(row.UpdatedAt)
		p.ConsentActor, p.ConsentTime = &actor, &at
	}
	if row.Enabled {
		var scope entity.KnowledgeAnswerScope
		if entity.KnowledgeAnswerDecode([]byte(row.ScopeJSON), "Scope", &scope) != nil || lexicalSHA([]byte(row.ScopeJSON)) != row.ScopeSHA256 {
			return p, answerFailure(503, "unavailable")
		}
		p.Scope, p.ScopeSHA256 = &scope, accessPtr(row.ScopeSHA256)
		p.Model = &entity.KnowledgeAnswerModelRef{ID: row.ModelID, SHA256: row.ModelSHA256}
	}
	return p, nil
}

func answerPolicyRow(tx *gorm.DB, domain string) (entity.KnowledgeAnswerPolicyRecord, error) {
	var row entity.KnowledgeAnswerPolicyRecord
	err := tx.Where("domain_id = ?", domain).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return entity.KnowledgeAnswerPolicyRecord{DomainID: domain}, nil
	}
	return row, err
}

// The tenant read epoch is locked first, but never incremented by an owner
// generation-policy change. Existing read Grants and MCP credentials survive it.
func (r *answerRunner) locked(ctx context.Context, owner, domain string, fn func(*gorm.DB, entity.KnowledgeAnswerPolicyRecord) error) error {
	return dao.KnowledgeAccessTransaction(ctx, dao.DB, func(tx *gorm.DB) error {
		if _, _, err := lexicalLock(ctx, tx, owner, domain); err != nil {
			return err
		}
		row, err := answerPolicyRow(tx.Clauses(clause.Locking{Strength: "UPDATE"}), domain)
		if err != nil {
			return err
		}
		if row.Version != 0 && row.OwnerID != owner {
			return answerFailure(404, "not_found")
		}
		return fn(tx, row)
	})
}

func (r *answerRunner) current(ctx context.Context, owner, domain string) (entity.KnowledgeAnswerPolicy, error) {
	var p entity.KnowledgeAnswerPolicy
	err := r.locked(ctx, owner, domain, func(_ *gorm.DB, row entity.KnowledgeAnswerPolicyRecord) error {
		var err error
		p, err = answerPolicy(row)
		return err
	})
	return p, err
}

func (r *answerRunner) inspect(ctx context.Context, owner, domain string, req entity.KnowledgeAnswerInspectRequest) (*entity.KnowledgeAnswerInspect, error) {
	if err := r.checkScope(ctx, owner, domain, req.Scope); err != nil {
		return nil, err
	}
	m, err := r.resolve(ctx, owner)
	if err != nil {
		return nil, err
	}
	if m.ref.ID != req.ModelID {
		return nil, answerFailure(404, "not_found")
	}
	p, err := r.current(ctx, owner, domain)
	if err != nil {
		return nil, err
	}
	scope := normalizeAnswerScope(req.Scope)
	return &entity.KnowledgeAnswerInspect{PolicyVersion: p.Version, Scope: scope, ScopeSHA256: lexicalSHA([]byte(lexicalJSON(scope))), Model: m.ref, ModelName: m.name, Destination: m.destination, Readiness: "ready", Limits: entity.KnowledgeAnswerFixedLimits()}, nil
}

func (r *answerRunner) setPolicy(ctx context.Context, owner, domain string, req entity.KnowledgeAnswerPolicySetRequest) (entity.KnowledgeAnswerPolicy, error) {
	var result entity.KnowledgeAnswerPolicy
	if req.Enabled {
		inspect, err := r.inspect(ctx, owner, domain, entity.KnowledgeAnswerInspectRequest{Version: req.Version, Scope: *req.Scope, ModelID: req.Model.ID})
		if err != nil {
			return result, err
		}
		if inspect.Model != *req.Model {
			return result, answerFailure(412, "scope_changed")
		}
		scope := inspect.Scope
		req.Scope = &scope
	}
	digest := lexicalSHA([]byte(lexicalJSON(req)))
	err := r.locked(ctx, owner, domain, func(tx *gorm.DB, row entity.KnowledgeAnswerPolicyRecord) error {
		var previous entity.KnowledgeAnswerRequestRecord
		err := tx.Where("owner_id = ? AND domain_id = ? AND operation = ? AND key_sha256 = ?", owner, domain, "policy", lexicalSHA([]byte(req.Key))).Take(&previous).Error
		if err == nil {
			if previous.RequestSHA256 != digest {
				return answerFailure(409, "idempotency_conflict")
			}
			if previous.PolicyVersion != row.Version {
				return answerFailure(409, "version_conflict")
			}
			result, err = answerPolicy(row)
			return err
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if row.Version != req.ExpectedVersion || row.Version >= entity.KnowledgeAccessMaxGeneration {
			return answerFailure(409, "version_conflict")
		}
		if req.Enabled {
			m, err := r.resolve(ctx, owner)
			if err != nil {
				return err
			}
			if m.ref != *req.Model {
				return answerFailure(412, "scope_changed")
			}
			if err := r.checkScope(ctx, owner, domain, *req.Scope); err != nil {
				return err
			}
		}
		row.OwnerID, row.DomainID, row.Version, row.Enabled = owner, domain, row.Version+1, req.Enabled
		row.ScopeJSON, row.ScopeSHA256, row.ModelID, row.ModelSHA256 = "", "", "", ""
		if req.Enabled {
			row.ScopeJSON = lexicalJSON(req.Scope)
			row.ScopeSHA256 = lexicalSHA([]byte(row.ScopeJSON))
			row.ModelID, row.ModelSHA256 = req.Model.ID, req.Model.SHA256
		}
		row.ConsentActor, row.UpdatedAt = owner, time.Now().UTC()
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		result, err = answerPolicy(row)
		if err != nil {
			return err
		}
		return tx.Create(&entity.KnowledgeAnswerRequestRecord{ID: common.GenerateUUID(), OwnerID: owner, DomainID: domain, Operation: "policy", KeySHA256: lexicalSHA([]byte(req.Key)), RequestSHA256: digest, PolicyVersion: row.Version, ScopeSHA256: row.ScopeSHA256, State: "completed", CreatedAt: row.UpdatedAt, UpdatedAt: row.UpdatedAt}).Error
	})
	return result, err
}

func (r *answerRunner) validatePermit(ctx context.Context, owner, domain string, p entity.KnowledgeAnswerPolicy) (*answerModel, error) {
	if !p.Enabled || p.Scope == nil || p.Model == nil || p.ScopeSHA256 == nil {
		return nil, answerFailure(403, "model_use_denied")
	}
	m, err := r.resolve(ctx, owner)
	if err != nil {
		return nil, err
	}
	if m.ref != *p.Model {
		return nil, answerFailure(412, "scope_changed")
	}
	if err := r.checkScope(ctx, owner, domain, *p.Scope); err != nil {
		return nil, err
	}
	return m, nil
}

func (r *answerRunner) permitLocked(ctx context.Context, owner, domain string, row entity.KnowledgeAnswerPolicyRecord, expected entity.KnowledgeAnswerPolicy) error {
	p, err := answerPolicy(row)
	if err != nil {
		return err
	}
	if !p.Enabled {
		return answerFailure(403, "model_use_denied")
	}
	if p.Version != expected.Version || lexicalJSON(p) != lexicalJSON(expected) {
		return answerFailure(409, "version_conflict")
	}
	_, err = r.validatePermit(ctx, owner, domain, p)
	return err
}

func answerPrune(ctx context.Context) error {
	now := time.Now().UTC()
	if err := dao.DB.WithContext(ctx).Model(&entity.KnowledgeAnswerRequestRecord{}).Where("created_at < ? AND response_json <> ?", now.Add(-24*time.Hour), "").Update("response_json", "").Error; err != nil {
		return err
	}
	return dao.DB.WithContext(ctx).Where("created_at < ?", now.Add(-7*24*time.Hour)).Delete(&entity.KnowledgeAnswerRequestRecord{}).Error
}

func answerEncode(operation, requestID string, data any) ([]byte, error) {
	raw, err := json.Marshal(map[string]any{"operation": operation, "contract_version": entity.KnowledgeAnswerVersion, "request_id": requestID, "data": data})
	definition := map[string]string{"inspect": "InspectResponse", "getPolicy": "PolicyGetResponse", "setPolicy": "PolicySetResponse", "answer": "AnswerResponse"}[operation]
	if err != nil || entity.KnowledgeAnswerDecode(raw, definition, nil) != nil {
		return nil, answerFailure(503, "unavailable")
	}
	return raw, nil
}

func AnswerErrorResponse(operation, requestID string, err error) (int, []byte) {
	e := normalizeAnswerError(err)
	if status, ok := answerErrorStatuses[e.Code]; ok {
		e.Status = status
	} else {
		e.Status, e.Code, e.ID = 503, "unavailable", nil
	}
	raw, _ := json.Marshal(map[string]any{"operation": operation, "contract_version": entity.KnowledgeAnswerVersion, "request_id": requestID, "data": nil, "answer_id": e.ID, "code": e.Code, "message": e.Error(), "outcome_known": e.Code != "provider_outcome_unknown" && e.Code != "unavailable", "retry_same_key": false})
	return e.Status, raw
}

var answerErrorStatuses = map[string]int{
	"invalid_request": 400, "unauthenticated": 401, "not_found": 404,
	"model_use_denied": 403, "version_conflict": 409, "idempotency_conflict": 409,
	"scope_changed": 412, "index_stale": 412, "unsupported_model": 412,
	"model_output_invalid": 502, "provider_unavailable": 502, "provider_outcome_unknown": 502,
	"capacity_exceeded": 413, "rate_limited": 429, "unavailable": 503, "contract_mismatch": 412,
}

func (d *DatasetService) KnowledgeAnswer(ctx context.Context, owner, domain, operation, requestID string, raw []byte) (int, []byte) {
	ctx, cancel := context.WithTimeout(dao.KnowledgeAccessPrivateReadContext(ctx), 75*time.Second)
	defer cancel()
	r := newAnswerRunner(d)
	status, data, err := r.execute(ctx, owner, domain, operation, raw)
	if err == nil {
		var body []byte
		body, err = answerEncode(operation, requestID, data)
		if err == nil {
			return status, body
		}
	}
	return AnswerErrorResponse(operation, requestID, err)
}

func (r *answerRunner) execute(ctx context.Context, owner, domain, operation string, raw []byte) (int, any, error) {
	if !entity.ValidKnowledgeLexicalID(domain) {
		return 0, nil, answerFailure(404, "not_found")
	}
	if _, _, err := r.dataset.lexicalContext(ctx, owner, domain, false); err != nil {
		return 0, nil, err
	}
	if len(raw) > 65536 {
		return 0, nil, answerFailure(413, "capacity_exceeded")
	}
	var version struct {
		Version string `json:"contract_version"`
	}
	var checked json.RawMessage
	if entity.KnowledgeParsedDecode(raw, &checked) != nil || json.Unmarshal(raw, &version) != nil {
		return 0, nil, answerFailure(400, "invalid_request")
	}
	if version.Version != entity.KnowledgeAnswerVersion {
		return 0, nil, answerFailure(412, "contract_mismatch")
	}
	if err := answerPrune(ctx); err != nil {
		return 0, nil, err
	}
	switch operation {
	case "inspect":
		var req entity.KnowledgeAnswerInspectRequest
		if entity.KnowledgeAnswerDecode(raw, "InspectRequest", &req) != nil {
			break
		}
		data, err := r.inspect(ctx, owner, domain, req)
		return 200, data, err
	case "getPolicy":
		if entity.KnowledgeAnswerDecode(raw, "PolicyGetRequest", nil) != nil {
			break
		}
		data, err := r.current(ctx, owner, domain)
		return 200, data, err
	case "setPolicy":
		var req entity.KnowledgeAnswerPolicySetRequest
		if entity.KnowledgeAnswerDecode(raw, "PolicySetRequest", &req) != nil {
			break
		}
		data, err := r.setPolicy(ctx, owner, domain, req)
		return 200, data, err
	case "answer":
		var req entity.KnowledgeAnswerRequest
		if entity.KnowledgeAnswerDecode(raw, "AnswerRequest", &req) != nil || strings.TrimSpace(req.Question) == "" {
			break
		}
		return r.answer(ctx, owner, domain, req)
	}
	return 0, nil, answerFailure(400, "invalid_request")
}
