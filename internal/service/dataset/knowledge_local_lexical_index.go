package dataset

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component"
	"ragflow/internal/ingestion/task/indexdoc"
	"ragflow/internal/tokenizer"
	"sort"
	"strings"
	"unicode/utf8"
)

type lexicalEngine interface {
	PrepareKnowledgeLexical(context.Context, string, string, string) error
	CreateKnowledgeLexicalRows(context.Context, string, string, []map[string]any) error
	VerifyKnowledgeLexicalRows(context.Context, string, string, []map[string]any) error
}

func (d *DatasetService) CreateKnowledgeLocalIndex(ctx context.Context, user, domain string, req entity.KnowledgeLexicalCreateRequest) (*entity.KnowledgeLexicalJob, error) {
	kb, _, err := d.lexicalContext(ctx, user, domain, true)
	if err != nil {
		return nil, err
	}
	if req.Version != entity.KnowledgeLocalLexicalVersion {
		return nil, lexicalError(412, "contract_mismatch")
	}
	if !entity.ValidKnowledgeLexicalKey(req.Key) || !entity.ValidKnowledgeLexicalVersion(req.ExpectedVersion, false) || !entity.ValidKnowledgeLexicalVersion(req.ExpectedFoundationVersion, false) || req.Selection.Validate(domain) != nil {
		return nil, lexicalError(400, "invalid_request")
	}
	if d.docEngine == nil || d.docEngine.GetType() != "elasticsearch" {
		return nil, lexicalError(503, "unavailable")
	}
	if _, ok := d.docEngine.(lexicalEngine); !ok {
		return nil, lexicalError(503, "unavailable")
	}
	req.Selection, _ = lexicalSelection(req.Selection)
	_, selectionSHA := lexicalSelection(req.Selection)
	opID := lexicalDigest("knowledge-local-operation/1", user, domain, "index", req.Key)
	requestSHA := lexicalSHA([]byte(lexicalJSON(req)))
	var record entity.KnowledgeLexicalJobRecord
	err = dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		current, p, err := lexicalLock(ctx, tx, user, domain)
		if err != nil {
			return err
		}
		var op entity.KnowledgeLexicalOperationRecord
		err = tx.Where("id = ?", opID).Take(&op).Error
		if err == nil {
			if op.RequestSHA256 != requestSHA {
				return lexicalError(409, "idempotency_conflict")
			}
			return tx.Where("id = ? AND owner_id = ? AND domain_id = ?", op.Payload, user, domain).Take(&record).Error
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if !p.Enabled {
			return lexicalError(409, "policy_disabled")
		}
		if p.Version != req.ExpectedVersion || current.KnowledgePolicyVersion != req.ExpectedFoundationVersion || lexicalLanguage(current) != lexicalLanguage(kb) {
			return lexicalError(409, "policy_changed")
		}
		var active int64
		if err := tx.Model(&entity.KnowledgeLexicalJobRecord{}).Where("domain_id = ? AND state IN ?", domain, []string{"queued", "building"}).Count(&active).Error; err != nil {
			return err
		}
		if active >= 4 {
			return lexicalError(429, "capacity_exceeded")
		}
		record = entity.KnowledgeLexicalJobRecord{ID: common.GenerateUUID(), OwnerID: user, DomainID: domain, Key: req.Key, RequestSHA256: requestSHA, SelectionSHA256: selectionSHA, Selection: lexicalJSON(req.Selection), PolicyVersion: p.Version, FoundationVersion: current.KnowledgePolicyVersion, Language: lexicalLanguage(current), State: "queued", Diagnostic: "building"}
		record.SessionSHA256 = ctx.Value(lexicalSessionKey{}).(lexicalSession).SHA256
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		return tx.Create(&entity.KnowledgeLexicalOperationRecord{ID: opID, OwnerID: user, DomainID: domain, Kind: "index", Key: req.Key, RequestSHA256: requestSHA, Payload: record.ID}).Error
	})
	if err != nil {
		return nil, lexicalNormalize(err)
	}
	return d.lexicalJobView(ctx, record)
}
func (d *DatasetService) GetKnowledgeLocalIndexJob(ctx context.Context, user, domain, id string) (*entity.KnowledgeLexicalJob, error) {
	if _, _, err := d.lexicalContext(ctx, user, domain, false); err != nil {
		return nil, err
	}
	if !entity.ValidKnowledgeLexicalID(id) {
		return nil, lexicalError(404, "not_found")
	}
	var job entity.KnowledgeLexicalJobRecord
	if err := dao.DB.WithContext(ctx).Where("id = ? AND owner_id = ? AND domain_id = ? AND protocol = ?", id, user, domain, entity.KnowledgeLocalLexicalProtocol).Take(&job).Error; err != nil {
		return nil, lexicalNormalize(err)
	}
	return d.lexicalJobView(ctx, job)
}
func (d *DatasetService) lexicalJobView(ctx context.Context, job entity.KnowledgeLexicalJobRecord) (*entity.KnowledgeLexicalJob, error) {
	if job.Protocol != "" && job.Protocol != entity.KnowledgeLocalLexicalProtocol {
		return nil, lexicalError(404, "not_found")
	}
	var selection entity.KnowledgeLexicalSelection
	if json.Unmarshal([]byte(job.Selection), &selection) != nil || selection.Validate(job.DomainID) != nil {
		return nil, lexicalError(409, "integrity_error")
	}
	_, sha := lexicalSelection(selection)
	if sha != job.SelectionSHA256 {
		return nil, lexicalError(409, "integrity_error")
	}
	current, err := d.lexicalCurrentness(ctx, job.OwnerID, job.DomainID, selection)
	if err != nil {
		return nil, err
	}
	readiness := lexicalReadiness(selection, sha, "building", current, "building")
	if job.State == "failed" {
		kind := "failed"
		if job.Diagnostic == "coverage_gap" {
			kind = "coverage_gap"
		}
		readiness = lexicalReadiness(selection, sha, kind, current, job.Diagnostic)
		if job.Diagnostic == "coverage_gap" {
			var m lexicalManifest
			if json.Unmarshal([]byte(job.Manifest), &m) != nil || m.Coverage.Complete || len(m.Coverage.Missing) == 0 {
				return nil, lexicalError(409, "integrity_error")
			}
			readiness.Coverage = &m.Coverage
		}
	}
	var index *entity.KnowledgeLexicalIndexRef
	if job.State == "published" {
		var publication entity.KnowledgeLexicalPublicationRecord
		if err := dao.DB.WithContext(ctx).Where("id = ? AND owner_id = ? AND domain_id = ?", job.IndexID, job.OwnerID, job.DomainID).Take(&publication).Error; err != nil {
			return nil, lexicalNormalize(err)
		}
		m, err := lexicalPublication(publication)
		if err != nil {
			return nil, err
		}
		index = &entity.KnowledgeLexicalIndexRef{ID: publication.ID, ManifestSHA256: publication.ManifestSHA256}
		readiness = lexicalReadiness(selection, sha, "ready", current, "")
		readiness.Publication = "published"
		readiness.Index = index
		readiness.Coverage = &m.Coverage
	}
	if job.State != "queued" && job.State != "building" && job.State != "failed" && job.State != "published" {
		return nil, lexicalError(409, "integrity_error")
	}
	if _, _, err := d.lexicalContext(ctx, job.OwnerID, job.DomainID, false); err != nil {
		return nil, err
	}
	return &entity.KnowledgeLexicalJob{ID: job.ID, Key: job.Key, RequestSHA256: job.RequestSHA256, State: job.State, Selection: selection, PolicyVersion: job.PolicyVersion, FoundationVersion: job.FoundationVersion, Index: index, Readiness: readiness, CreatedAt: lexicalTime(job.CreatedAt), UpdatedAt: lexicalTime(job.UpdatedAt)}, nil
}

type lexicalTokenInput struct {
	ID              string
	Binding         string
	Reference       string
	ReferenceSHA256 string
	TextSHA256      string
	SourceID        string
	ByteLength      int64
	StartByte       int64
	Text            string
	Name            string
}

func lexicalTokenize(ctx context.Context, job entity.KnowledgeLexicalJobRecord, units []lexicalUnit) ([]map[string]any, int64, error) {
	inputs := make([]lexicalTokenInput, len(units))
	for i, unit := range units {
		inputs[i] = lexicalTokenInput{ID: unit.Row.ID, Binding: unit.Row.Binding, Reference: unit.Row.Reference, ReferenceSHA256: unit.Row.ReferenceSHA256, TextSHA256: unit.Row.TextSHA256, SourceID: unit.Row.Target.Source.SourceID, ByteLength: unit.Row.ByteLength, StartByte: unit.Row.Member.Range.Start, Text: unit.Text, Name: unit.Name}
	}
	return lexicalTokenizeInputs(ctx, job, inputs)
}

func lexicalTokenizeInputs(ctx context.Context, job entity.KnowledgeLexicalJobRecord, units []lexicalTokenInput) ([]map[string]any, int64, error) {
	if len(units) > 32768 {
		return nil, 0, lexicalError(413, "capacity_exceeded")
	}
	stage, err := component.NewTokenizerComponent(map[string]any{"search_method": []string{"full_text"}, "fields": []string{"text"}})
	if err != nil {
		return nil, 0, err
	}
	// Reading order belongs to each fixed binding, independent of the selected set.
	byBinding := make(map[string][]int)
	for i, unit := range units {
		byBinding[unit.Binding] = append(byBinding[unit.Binding], i)
	}
	orderByInput := make([]int, len(units))
	for _, indices := range byBinding {
		sort.Slice(indices, func(i, j int) bool {
			left, right := units[indices[i]], units[indices[j]]
			if left.StartByte != right.StartByte {
				return left.StartByte < right.StartByte
			}
			return left.ID < right.ID
		})
		for order, i := range indices {
			orderByInput[i] = order
		}
	}
	rows := make([]map[string]any, 0, len(units))
	var tokenless int64
	for i, unit := range units {
		if !entity.ValidKnowledgeLexicalHash(unit.ID) || !entity.ValidKnowledgeLexicalHash(unit.Binding) || !entity.ValidKnowledgeLexicalID(unit.SourceID) || unit.StartByte < 0 || unit.ByteLength != int64(len(unit.Text)) || unit.ByteLength > 8192 || !utf8.ValidString(unit.Text) || !utf8.ValidString(unit.Name) || len(unit.Reference) == 0 || len(unit.Reference) > 64<<10 || !utf8.ValidString(unit.Reference) || lexicalSHA([]byte(unit.Text)) != unit.TextSHA256 || lexicalSHA([]byte(unit.Reference)) != unit.ReferenceSHA256 {
			return nil, 0, lexicalError(409, "integrity_error")
		}
		out, err := stage.Invoke(ctx, dao.DB, map[string]any{"name": unit.Name, "tenant_id": "", "kb_id": job.DomainID, "lang": job.Language, "output_format": "chunks", "chunks": []map[string]any{{"text": unit.Text}}})
		if err != nil {
			return nil, 0, err
		}
		chunks, ok := out["chunks"].([]map[string]any)
		if !ok || len(chunks) != 1 {
			return nil, 0, lexicalError(409, "integrity_error")
		}
		row := chunks[0]
		if row["text"] != unit.Text {
			return nil, 0, lexicalError(409, "integrity_error")
		}
		for k := range row {
			if strings.HasPrefix(k, "q_") && strings.HasSuffix(k, "_vec") || k == "mom_id" {
				return nil, 0, lexicalError(409, "integrity_error")
			}
		}
		if out["embedding_token_consumption"] != nil {
			return nil, 0, lexicalError(409, "integrity_error")
		}
		if strings.TrimSpace(fmt.Sprint(row["content_ltks"])) == "" {
			tokenless += unit.ByteLength
		}
		row["id"] = unit.ID
		row["knowledge_binding_kwd"] = unit.Binding
		row["knowledge_ref"] = unit.Reference
		row["available_int"] = 0
		row["chunk_order_int"] = orderByInput[i]
		if _, err := indexdoc.ProcessChunksForPipeline(chunks, unit.SourceID, unit.Name, job.CreatedAt); err != nil {
			return nil, 0, err
		}
		row["kb_id"] = job.DomainID
		if row["content_with_weight"] != unit.Text {
			return nil, 0, lexicalError(409, "integrity_error")
		}
		rows = append(rows, row)
	}
	return rows, tokenless, nil
}

func lexicalPublication(p entity.KnowledgeLexicalPublicationRecord) (*lexicalManifest, error) {
	if lexicalSHA([]byte(p.Payload)) != p.ManifestSHA256 || lexicalDigest("knowledge-local-index-publication/1", p.OwnerID, p.DomainID, p.ManifestSHA256) != p.ID {
		return nil, lexicalError(409, "integrity_error")
	}
	var m lexicalManifest
	if json.Unmarshal([]byte(p.Payload), &m) != nil || lexicalJSON(m) != p.Payload || m.Version != entity.KnowledgeLocalLexicalVersion || m.Owner != p.OwnerID || m.Domain != p.DomainID || m.Selection.Validate(p.DomainID) != nil || !m.Coverage.Complete || len(m.Coverage.Missing) != 0 || m.Coverage.IndexedUnits != len(m.Rows) || m.Coverage.SelectedSources != len(m.Selection.Targets) || m.Coverage.CoveredSources != len(m.Selection.Targets) || m.Coverage.CoveredBytes != m.Coverage.ExpectedBytes || len(m.Rows) > 32768 {
		return nil, lexicalError(409, "integrity_error")
	}
	_, sha := lexicalSelection(m.Selection)
	if sha != m.SelectionSHA256 || sha != p.SelectionSHA256 || m.Profile != lexicalProfile(m.Language, m.Producer) {
		return nil, lexicalError(409, "integrity_error")
	}
	seen := map[string]bool{}
	targets := map[string]string{}
	for _, t := range m.Selection.Targets {
		targets[t.Source.SourceID] = lexicalJSON(t)
	}
	var total int64
	for _, r := range m.Rows {
		strategy := "c1_complete_root"
		if r.Target.Representation.Kind == "structure_view" {
			strategy = "r1_existing_passage"
		}
		if seen[r.ID] || targets[r.Target.Source.SourceID] != lexicalJSON(r.Target) || r.Plan != lexicalPlan(strategy, m.Language, m.Producer) || r.Binding != lexicalBinding(r.Target, r.Plan) || r.ReferenceSHA256 != lexicalSHA([]byte(r.Reference)) || r.ByteLength < 0 || r.ByteLength > 8192 {
			return nil, lexicalError(409, "integrity_error")
		}
		seen[r.ID] = true
		total += r.ByteLength
	}
	if total != m.Coverage.CoveredBytes {
		return nil, lexicalError(409, "integrity_error")
	}
	return &m, nil
}

func (d *DatasetService) lexicalExpectedRows(ctx context.Context, m *lexicalManifest) ([]map[string]any, error) {
	units, coverage, _, err := d.lexicalUnits(ctx, m.Owner, m.Domain, m.Language, m.Producer, m.Selection)
	if err != nil {
		return nil, err
	}
	if !coverage.Complete || coverage.CoveredBytes != m.Coverage.CoveredBytes || len(units) != len(m.Rows) {
		return nil, lexicalError(409, "integrity_error")
	}
	rows := make([]map[string]any, 0, len(m.Rows))
	for i, u := range units {
		r := m.Rows[i]
		if lexicalJSON(r) != lexicalJSON(u.Row) {
			return nil, lexicalError(409, "integrity_error")
		}
		rows = append(rows, map[string]any{"id": r.ID, "kb_id": m.Domain, "doc_id": r.Target.Source.SourceID, "knowledge_binding_kwd": r.Binding, "knowledge_ref": r.Reference, "content_with_weight": u.Text, "available_int": 0})
	}
	return rows, nil
}

func (d *DatasetService) InspectKnowledgeLocalIndex(ctx context.Context, user, domain string, req entity.KnowledgeLexicalInspectRequest) (*entity.KnowledgeLexicalReadinessData, error) {
	kb, p, err := d.lexicalContext(ctx, user, domain, false)
	if err != nil {
		return nil, err
	}
	if req.Version != entity.KnowledgeLocalLexicalVersion {
		return nil, lexicalError(412, "contract_mismatch")
	}
	if req.Selection.Validate(domain) != nil {
		return nil, lexicalError(400, "invalid_request")
	}
	s, sha := lexicalSelection(req.Selection)
	producer, err := tokenizer.ProducerSHA256()
	if err != nil {
		return nil, lexicalNormalize(err)
	}
	_, _, current, err := d.lexicalUnits(ctx, user, domain, lexicalLanguage(kb), producer, s)
	if err != nil {
		return nil, err
	}
	r := lexicalReadiness(s, sha, "unindexed", current, "none")
	var published entity.KnowledgeLexicalPublicationRecord
	err = dao.DB.WithContext(ctx).Where("owner_id = ? AND domain_id = ? AND selection_sha256 = ? AND protocol = ?", user, domain, sha, entity.KnowledgeLocalLexicalProtocol).Order("created_at DESC, id DESC").Take(&published).Error
	if err == nil {
		m, err := lexicalPublication(published)
		if err != nil {
			return nil, err
		}
		r = lexicalReadiness(s, sha, "ready", current, "")
		r.Publication = "published"
		r.Index = &entity.KnowledgeLexicalIndexRef{ID: published.ID, ManifestSHA256: published.ManifestSHA256}
		r.Coverage = &m.Coverage
		if !p.Enabled {
			r.Kind = "stale"
			r.Diagnostics = []string{"policy_disabled"}
		} else if m.PolicyVersion != p.Version || m.FoundationVersion != p.FoundationVersion {
			r.Kind = "stale"
			r.Diagnostics = []string{"policy_changed"}
		} else if m.Language != lexicalLanguage(kb) || m.Producer != producer {
			r.Kind = "stale"
			r.Diagnostics = []string{"profile_changed"}
		}
		if r.Kind == "ready" {
			native, ok := d.docEngine.(lexicalEngine)
			if !ok {
				return nil, lexicalError(503, "unavailable")
			}
			rows, err := d.lexicalExpectedRows(ctx, m)
			if err != nil {
				return nil, err
			}
			if err := native.VerifyKnowledgeLexicalRows(ctx, "ragflow_"+kb.TenantID, domain, rows); err != nil {
				return nil, lexicalError(409, "integrity_error")
			}
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, lexicalNormalize(err)
	} else {
		var job entity.KnowledgeLexicalJobRecord
		jobErr := dao.DB.WithContext(ctx).Where("owner_id = ? AND domain_id = ? AND selection_sha256 = ? AND protocol = ?", user, domain, sha, entity.KnowledgeLocalLexicalProtocol).Order("created_at DESC, id DESC").Take(&job).Error
		if jobErr == nil {
			view, err := d.lexicalJobView(ctx, job)
			if err != nil {
				return nil, err
			}
			r = view.Readiness
			r.Currentness = current
		} else if !errors.Is(jobErr, gorm.ErrRecordNotFound) {
			return nil, lexicalNormalize(jobErr)
		}
		if !p.Enabled {
			r = lexicalReadiness(s, sha, "unindexed", current, "policy_disabled")
		}
	}
	finalKB, finalPolicy, err := d.lexicalContext(ctx, user, domain, false)
	if err != nil {
		return nil, err
	}
	finalCurrent, err := d.lexicalCurrentness(ctx, user, domain, s)
	if err != nil {
		return nil, err
	}
	r.Currentness = finalCurrent
	if finalPolicy.Version != p.Version || finalPolicy.FoundationVersion != p.FoundationVersion || finalPolicy.Enabled != p.Enabled || lexicalLanguage(finalKB) != lexicalLanguage(kb) {
		diagnostic := "policy_changed"
		if !finalPolicy.Enabled {
			diagnostic = "policy_disabled"
		} else if lexicalLanguage(finalKB) != lexicalLanguage(kb) {
			diagnostic = "profile_changed"
		}
		if r.Index != nil {
			r.Kind = "stale"
			r.Diagnostics = []string{diagnostic}
		} else {
			r = lexicalReadiness(s, sha, "unindexed", finalCurrent, diagnostic)
		}
	}
	p = finalPolicy
	return &entity.KnowledgeLexicalReadinessData{Policy: p, Readiness: r}, nil
}
