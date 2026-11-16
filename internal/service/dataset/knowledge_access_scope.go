package dataset

import (
	"context"
	"errors"
	"sort"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/storage"
	"ragflow/internal/tokenizer"
)

type contentProofStoreKey struct{}

// This context value selects the current transaction for pure proof storage
// reads. It never bypasses an owner check in any public owner-only entry.
func accessProofContext(ctx context.Context, tx *gorm.DB) context.Context {
	return context.WithValue(storage.WithPrivateRead(ctx), contentProofStoreKey{}, tx.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}))
}
func contentProofStore(ctx context.Context) *gorm.DB {
	if tx, ok := ctx.Value(contentProofStoreKey{}).(*gorm.DB); ok {
		return tx.WithContext(ctx)
	}
	return dao.DB.WithContext(ctx)
}

func accessContentTarget(value entity.KnowledgeAccessTarget) entity.KnowledgeContentTarget {
	id := value.Representation.ID
	return entity.KnowledgeContentTarget{Source: entity.KnowledgeContentSourceRef{DomainID: value.Source.DomainID, SourceID: value.Source.SourceID, RevisionID: value.Source.SourceRevisionID, SHA256: value.Source.SourceSHA256}, Representation: entity.KnowledgeContentRepresentationRef{Kind: value.Representation.Kind, ID: &id, ProducerSHA256: value.Representation.ProducerSHA256}}
}
func accessTarget(value entity.KnowledgeContentTarget) entity.KnowledgeAccessTarget {
	return entity.KnowledgeAccessTarget{Source: entity.KnowledgeAccessSource{DomainID: value.Source.DomainID, SourceID: value.Source.SourceID, SourceRevisionID: value.Source.RevisionID, SourceSHA256: value.Source.SHA256}, Representation: entity.KnowledgeAccessRepresentation{Kind: value.Representation.Kind, ID: *value.Representation.ID, ProducerSHA256: value.Representation.ProducerSHA256}}
}

func accessCurrentPolicies(ctx context.Context, tx *gorm.DB, kb *entity.Knowledgebase) (contentLexicalPolicyCapture, error) {
	local, err := lexicalPolicyRow(ctx, accessLocked(tx, true), kb.ID)
	if err != nil {
		return contentLexicalPolicyCapture{}, err
	}
	content, err := parsedPolicyRecord(accessLocked(tx, true), kb.CreatedBy, kb.ID)
	if err != nil {
		return contentLexicalPolicyCapture{}, err
	}
	if local.OwnerID != kb.CreatedBy || !local.Enabled || !content.LexicalEnabled || local.Version < 1 || local.Version > entity.KnowledgeAccessMaxGeneration {
		return contentLexicalPolicyCapture{}, accessError(409, "index_stale")
	}
	return contentLexicalPolicyCapture{FoundationVersion: kb.KnowledgePolicyVersion, LocalVersion: local.Version, ContentVersion: content.Version, Language: lexicalLanguage(kb), LocalEnabled: local.Enabled, ContentEnabled: content.LexicalEnabled}, nil
}

func (d *DatasetService) accessPublication(ctx context.Context, tx *gorm.DB, kb *entity.Knowledgebase, scope entity.KnowledgeAccessModuleScope, current bool) (*contentLexicalManifest, error) {
	if len(scope.Targets) < 1 || len(scope.Targets) > 32 {
		return nil, accessError(400, "invalid_request")
	}
	var record entity.KnowledgeLexicalPublicationRecord
	if err := tx.Where("id = ? AND owner_id = ? AND domain_id = ? AND protocol = ?", scope.Publication.ID, kb.CreatedBy, kb.ID, entity.KnowledgeContentLexicalProtocol).Take(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, accessError(409, "index_unindexed")
		}
		return nil, err
	}
	if record.ManifestSHA256 != scope.Publication.ManifestSHA256 {
		return nil, accessError(409, "integrity_error")
	}
	manifest, err := contentLexicalPublication(record)
	if err != nil {
		return nil, err
	}
	available := map[string]bool{}
	for _, target := range manifest.Selection.Targets {
		available[accessJSON(accessTarget(target))] = true
	}
	seen := map[string]bool{}
	selection := entity.KnowledgeContentLexicalSelection{Kind: "fixed_representations", Targets: []entity.KnowledgeContentTarget{}}
	for _, target := range scope.Targets {
		if target.Source.DomainID != kb.ID || seen[target.Source.SourceID] || !available[accessJSON(target)] {
			return nil, accessError(409, "scope_not_supported")
		}
		seen[target.Source.SourceID] = true
		selection.Targets = append(selection.Targets, accessContentTarget(target))
	}
	if !current {
		return manifest, nil
	}
	// Final exact target membership is established above. Complete metadata
	// closure must precede lifecycle fences and every derived body read below.
	if err = d.accessDependencyClosure(accessProofContext(ctx, tx), kb, scope.Targets); err != nil {
		return nil, err
	}
	policies, err := accessCurrentPolicies(ctx, tx, kb)
	if err != nil {
		return nil, err
	}
	if policies != manifest.Policies {
		return nil, accessError(409, "index_stale")
	}
	producer, err := tokenizer.ProducerSHA256()
	if err != nil {
		return nil, err
	}
	if producer != manifest.ProducerSHA256 || manifest.ProfileSHA256 != contentLexicalProfile(policies.Language, producer).SHA256 {
		return nil, accessError(409, "index_stale")
	}
	// Source selection precedes all lifecycle/representation-body materialization.
	for _, target := range selection.Targets {
		data, err := loadLifecycleSource(accessLocked(tx, true), kb, target.Source.SourceID)
		if err != nil {
			return nil, err
		}
		if data.state.State != "active" {
			return nil, accessError(409, "scope_changed")
		}
	}
	fences, err := contentLexicalCaptureSources(accessLocked(tx, true), kb, selection)
	if err != nil {
		return nil, err
	}
	published := map[string]string{}
	for _, fence := range manifest.Sources {
		published[fence.SourceID] = fence.SHA256
	}
	for _, fence := range fences {
		if published[fence.SourceID] != fence.SHA256 {
			return nil, accessError(409, "index_stale")
		}
	}
	return manifest, nil
}

// accessDependency is a private proof footprint, not a selectable wire scope.
// A source-only contribution requires complete source_revision coverage; an
// exact contribution requires that exact complete Target in the final set.
type accessDependency struct {
	Source   entity.KnowledgeAccessSource
	Target   *entity.KnowledgeAccessTarget
	Complete bool
}

func accessVerifyClosure(selected []entity.KnowledgeAccessTarget, proofs map[string][]accessDependency) error {
	exact := map[string]bool{}
	sources := map[entity.KnowledgeAccessSource]bool{}
	for _, target := range selected {
		exact[accessJSON(target)] = true
		if target.Representation.Kind == "source_revision" {
			sources[target.Source] = true
		}
	}
	visited := map[string]bool{}
	visiting := map[string]bool{}
	var visit func(string) error
	visit = func(key string) error {
		if visited[key] {
			return nil
		}
		if visiting[key] {
			return nil
		}
		deps, ok := proofs[key]
		if !ok || len(deps) == 0 {
			return accessError(409, "scope_not_supported")
		}
		visiting[key] = true
		for _, dep := range deps {
			if !dep.Complete {
				return accessError(409, "scope_not_supported")
			}
			if dep.Target == nil {
				if !sources[dep.Source] {
					return accessError(409, "scope_not_supported")
				}
				continue
			}
			other := accessJSON(*dep.Target)
			if dep.Target.Source != dep.Source || !exact[other] {
				return accessError(409, "scope_not_supported")
			}
			if other != key {
				if err := visit(other); err != nil {
					return err
				}
			}
		}
		delete(visiting, key)
		visited[key] = true
		return nil
	}
	for key := range exact {
		if err := visit(key); err != nil {
			return err
		}
	}
	return nil
}

func (d *DatasetService) accessDependencyClosure(ctx context.Context, kb *entity.Knowledgebase, selected []entity.KnowledgeAccessTarget) error {
	proofs := map[string][]accessDependency{}
	for _, target := range selected {
		meta, err := d.contentStoredRepresentation(ctx, kb, accessContentTarget(target))
		if err != nil {
			return err
		}
		if meta.RootRef == nil || meta.Source == nil || *meta.Source != accessContentTarget(target).Source || meta.State == "unavailable" {
			return accessError(409, "scope_not_supported")
		}
		// The four frozen owning codecs prove single-source reading assembly.
		// R1 members are independently fixed to their source; R2/parsed digests
		// contain one exact original/source and closed local payload references.
		switch target.Representation.Kind {
		case "source_revision", "structure_view", "document_representation", "parsed_representation":
		default:
			return accessError(409, "scope_not_supported")
		}
		copy := target
		proofs[accessJSON(target)] = []accessDependency{{Source: target.Source, Target: &copy, Complete: true}}
	}
	return accessVerifyClosure(selected, proofs)
}

func accessSelectedRows(manifest *contentLexicalManifest, targets []entity.KnowledgeAccessTarget) ([]contentLexicalManifestRow, [][]contentLexicalManifestRow, error) {
	allowed := map[string]int{}
	for i, target := range targets {
		key := accessJSON(accessContentTarget(target))
		if _, ok := allowed[key]; ok {
			return nil, nil, accessError(409, "integrity_error")
		}
		allowed[key] = i
	}
	rows := []contentLexicalManifestRow{}
	byTarget := make([][]contentLexicalManifestRow, len(targets))
	for i := range byTarget {
		byTarget[i] = []contentLexicalManifestRow{}
	}
	for _, row := range manifest.Rows {
		if index, ok := allowed[accessJSON(row.Reference.Address.Target)]; ok {
			rows = append(rows, row)
			byTarget[index] = append(byTarget[index], row)
		}
	}
	if len(rows) > 32768 {
		return nil, nil, accessError(413, "capacity_exceeded")
	}
	for i := range byTarget {
		sort.Slice(byTarget[i], func(a, b int) bool {
			left, right := byTarget[i][a], byTarget[i][b]
			if left.Reference.Range.StartByte != right.Reference.Range.StartByte {
				return left.Reference.Range.StartByte < right.Reference.Range.StartByte
			}
			if left.Reference.Range.EndByte != right.Reference.Range.EndByte {
				return left.Reference.Range.EndByte < right.Reference.Range.EndByte
			}
			if *left.Reference.Address.Node.ID != *right.Reference.Address.Node.ID {
				return *left.Reference.Address.Node.ID < *right.Reference.Address.Node.ID
			}
			return left.ID < right.ID
		})
	}
	return rows, byTarget, nil
}

type accessCapture struct {
	OwnerID        string                      `json:"storage_owner_id"`
	Mode           string                      `json:"mode"`
	Delivery       string                      `json:"delivery"`
	ScopeSHA256    string                      `json:"module_scope_sha256"`
	SelectedSHA256 string                      `json:"selected_sha256"`
	ShareVersion   int64                       `json:"sharing_policy_version"`
	Policies       contentLexicalPolicyCapture `json:"policies"`
	Fences         []contentLexicalSourceFence `json:"sources"`
}
type verifiedAccessActor struct {
	UserID   string
	View     entity.KnowledgeAccessViewRecord
	KB       *entity.Knowledgebase
	Scope    entity.KnowledgeAccessModuleScope
	Capture  accessCapture
	Manifest *contentLexicalManifest
	Rows     []contentLexicalManifestRow
	ByTarget [][]contentLexicalManifestRow
	AuditID  string
}

func (d *DatasetService) accessValidateView(ctx context.Context, tx *gorm.DB, actor string, view entity.KnowledgeAccessViewRecord, epoch int64, now time.Time) (*verifiedAccessActor, error) {
	if view.SubjectUserID != actor || view.CredentialID != nil {
		return nil, accessError(404, "not_found")
	}
	if !now.Before(view.ExpiresAt) || view.TenantEpoch != epoch {
		return nil, accessError(409, "view_stale")
	}
	kb, err := accessDomain(ctx, tx, actor, view.DomainID, false, true)
	if err != nil {
		return nil, err
	}
	if kb.TenantID != view.TenantID {
		return nil, accessError(409, "scope_changed")
	}
	module, whole, err := accessModule(tx, kb, view.ModuleID, true)
	if err != nil {
		return nil, err
	}
	if !module.Enabled || module.Version != view.ModuleVersion {
		return nil, accessError(409, "view_stale")
	}
	var scope entity.KnowledgeAccessModuleScope
	var capture accessCapture
	if len(view.PrivateManifestJSON)+len(view.CaptureJSON) > 65536 || entity.KnowledgeAccessDecode([]byte(view.PrivateManifestJSON), "ModuleScope", &scope) != nil || entity.KnowledgeParsedDecode([]byte(view.CaptureJSON), &capture) != nil || capture.OwnerID != kb.CreatedBy || capture.ScopeSHA256 != module.ScopeSHA256 || capture.SelectedSHA256 != lexicalSHA([]byte(view.PrivateManifestJSON)) {
		return nil, accessError(409, "view_stale")
	}
	if scope.Publication != whole.Publication {
		return nil, accessError(409, "view_stale")
	}
	allowed := map[string]bool{}
	for _, target := range whole.Targets {
		allowed[accessJSON(target)] = true
	}
	for _, target := range scope.Targets {
		if !allowed[accessJSON(target)] {
			return nil, accessError(409, "scope_changed")
		}
	}
	var grant entity.KnowledgeAccessGrantRecord
	if err := accessLocked(tx, true).Where("id = ? AND tenant_id = ? AND domain_id = ? AND module_id = ? AND module_version = ? AND revoked_at IS NULL AND expires_at > ?", view.GrantID, kb.TenantID, kb.ID, module.ID, module.Version, now).Take(&grant).Error; err != nil {
		return nil, accessError(409, "view_stale")
	}
	if err := accessGrantIntegrity(tx, grant); err != nil {
		return nil, err
	}
	matches, err := accessGrantSubject(tx, grant, actor)
	if err != nil {
		return nil, err
	}
	if grant.IssuedBy != kb.CreatedBy || !matches || !accessCovers(grant, capture.Mode, capture.Delivery) || view.ExpiresAt.After(grant.ExpiresAt) {
		return nil, accessError(409, "view_stale")
	}
	policy, err := accessPolicy(tx, kb, true)
	if err != nil {
		return nil, err
	}
	if !policy.Enabled || policy.Version != capture.ShareVersion {
		return nil, accessError(409, "view_stale")
	}
	proofCtx := accessProofContext(ctx, tx)
	manifest, err := d.accessPublication(proofCtx, tx, kb, scope, true)
	if err != nil {
		return nil, err
	}
	if manifest.Policies != capture.Policies {
		return nil, accessError(409, "view_stale")
	}
	selection := entity.KnowledgeContentLexicalSelection{Kind: "fixed_representations", Targets: []entity.KnowledgeContentTarget{}}
	for _, target := range scope.Targets {
		selection.Targets = append(selection.Targets, accessContentTarget(target))
	}
	fences, err := contentLexicalCaptureSources(accessLocked(tx, true), kb, selection)
	if err != nil {
		return nil, err
	}
	if accessJSON(fences) != accessJSON(capture.Fences) {
		return nil, accessError(409, "view_stale")
	}
	rows, byTarget, err := accessSelectedRows(manifest, scope.Targets)
	if err != nil {
		return nil, err
	}
	return &verifiedAccessActor{UserID: actor, View: view, KB: kb, Scope: scope, Capture: capture, Manifest: manifest, Rows: rows, ByTarget: byTarget}, nil
}
