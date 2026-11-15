package dataset

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func structureReadError(status int, code string) error {
	return knowledgeError(status, code, "Structure operation could not be completed")
}

func structureReadNormalize(err error) error {
	if err == nil {
		return nil
	}
	var public *KnowledgeError
	if errors.As(err, &public) {
		if public.Status == 404 {
			return structureReadError(404, "not_found")
		}
		if public.Status == 400 {
			return structureReadError(400, "invalid_request")
		}
		if slices.Contains([]string{"not_found", "invalid_request", "structure_integrity", "source_unavailable", "unavailable"}, public.Code) {
			return err
		}
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return structureReadError(404, "not_found")
	}
	return structureReadError(503, "unavailable")
}

// AuthorizeKnowledgeStructure uses the existing owner, tenant and domain checks.
func (d *DatasetService) AuthorizeKnowledgeStructure(ctx context.Context, user, domain string) error {
	_, err := d.knowledgeBase(ctx, user, domain, true)
	return structureReadNormalize(err)
}

func (d *DatasetService) structureReadView(ctx context.Context, user, domain, id string) (*entity.KnowledgeStructureView, error) {
	if err := d.AuthorizeKnowledgeStructure(ctx, user, domain); err != nil {
		return nil, err
	}
	return d.structureStoredView(ctx, user, domain, id)
}

// structureStoredView verifies immutable storage metadata without conferring authority.
func (d *DatasetService) structureStoredView(ctx context.Context, user, domain, id string) (*entity.KnowledgeStructureView, error) {
	if !isKnowledgeID(id, 64) {
		return nil, structureReadError(404, "not_found")
	}
	var row entity.KnowledgeStructureViewRecord
	if err := contentProofStore(ctx).Where("id = ? AND owner_id = ? AND domain_id = ? AND published = ?", id, user, domain, true).Take(&row).Error; err != nil {
		return nil, structureReadNormalize(err)
	}
	var view entity.KnowledgeStructureView
	if json.Unmarshal([]byte(row.Payload), &view) != nil || view.ViewID != id || view.DomainID != domain || view.OwnerID != user || len(view.Members) < 1 || len(view.Members) > 32 {
		return nil, structureReadError(503, "structure_integrity")
	}
	// No metadata, diagnostics or counts escape until every selected fixed source
	// has passed current authorization. Archiving does not revoke history.
	for _, member := range view.Members {
		source, err := d.knowledgeSource(ctx, domain, member.SourceRevisionID)
		if err != nil {
			return nil, structureReadNormalize(err)
		}
		if source.DocumentID != member.SourceID || source.SHA256 != member.SHA256 || source.ByteLength != member.ByteLength || source.RootNodeID != member.SourceNodeID {
			return nil, structureReadError(503, "structure_integrity")
		}
	}
	return &view, nil
}

func (d *DatasetService) GetKnowledgeStructureView(ctx context.Context, user, domain, view string) (*entity.KnowledgeStructureView, error) {
	return d.structureReadView(ctx, user, domain, view)
}

func structureMember(view *entity.KnowledgeStructureView, source string) (*entity.KnowledgeStructureViewMember, error) {
	for i := range view.Members {
		if view.Members[i].SourceID == source {
			return &view.Members[i], nil
		}
	}
	return nil, structureReadError(404, "not_found")
}

func structureReadNode(ctx context.Context, user, domain string, view *entity.KnowledgeStructureView, id string) (*entity.KnowledgeStructureNode, error) {
	if !isKnowledgeID(id, 64) {
		return nil, structureReadError(404, "not_found")
	}
	var row entity.KnowledgeStructureNodeRecord
	if err := contentProofStore(ctx).Where("id = ? AND owner_id = ? AND domain_id = ? AND view_id = ?", id, user, domain, view.ViewID).Take(&row).Error; err != nil {
		return nil, structureReadNormalize(err)
	}
	var node entity.KnowledgeStructureNode
	if json.Unmarshal([]byte(row.Payload), &node) != nil || node.StructureNodeID != id || node.ViewID != view.ViewID {
		return nil, structureReadError(503, "structure_integrity")
	}
	member, err := structureMember(view, node.SourceID)
	if err != nil || member.SourceRevisionID != node.SourceRevisionID || member.SourceNodeID != node.SourceNodeID {
		return nil, structureReadError(503, "structure_integrity")
	}
	return &node, nil
}

func (d *DatasetService) GetKnowledgeStructureNode(ctx context.Context, user, domain, viewID, nodeID string) (*entity.KnowledgeStructureNode, error) {
	view, err := d.structureReadView(ctx, user, domain, viewID)
	if err != nil {
		return nil, err
	}
	return structureReadNode(ctx, user, domain, view, nodeID)
}

type structureReadCursor struct {
	Offset     int       `json:"offset"`
	LastID     string    `json:"last_id"`
	CapturedAt time.Time `json:"captured_at"`
	UpperID    string    `json:"upper_id"`
	UpperTime  time.Time `json:"upper_time"`
	ExpiresAt  time.Time `json:"expires_at"`
}

func structureLoadReadCursor(ctx context.Context, user, domain, scope, id string, limit int) (*structureReadCursor, error) {
	if limit < 1 || limit > 20 {
		return nil, structureReadError(400, "invalid_request")
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	if id == "" {
		return &structureReadCursor{CapturedAt: now, ExpiresAt: now.Add(600 * time.Second)}, nil
	}
	if !isKnowledgeID(id, 32) {
		return nil, structureReadError(400, "invalid_request")
	}
	var row entity.KnowledgeStructureCursorRecord
	err := dao.DB.WithContext(ctx).Where("id = ? AND owner_id = ? AND domain_id = ? AND scope_sha256 = ?", id, user, domain, scope).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, structureReadError(400, "invalid_request")
	}
	if err != nil {
		return nil, structureReadNormalize(err)
	}
	var cursor structureReadCursor
	if json.Unmarshal([]byte(row.Payload), &cursor) != nil || cursor.Offset < 0 || !row.ExpiresAt.Equal(cursor.ExpiresAt) || !now.Before(row.ExpiresAt) {
		return nil, structureReadError(400, "invalid_request")
	}
	return &cursor, nil
}

func structureSaveReadCursor(ctx context.Context, user, domain, scope string, value *structureReadCursor) (*string, error) {
	id := common.GenerateUUID()
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	err = dao.DB.WithContext(ctx).Create(&entity.KnowledgeStructureCursorRecord{ID: id, OwnerID: user, DomainID: domain, ScopeSHA256: scope, Payload: string(raw), ExpiresAt: value.ExpiresAt, CreatedAt: time.Now().UTC()}).Error
	if err != nil {
		return nil, structureReadNormalize(err)
	}
	return &id, nil
}

func structureReadScope(user, domain, kind string, values ...any) string {
	digest, _ := entity.KnowledgeLifecycleDigest(map[string]any{"kind": "knowledge-structure-cursor/0.1.0", "owner_id": user, "domain_id": domain, "operation": kind, "scope": values})
	return digest
}

func (d *DatasetService) ListKnowledgeStructureNodes(ctx context.Context, user, domain, viewID, source, parent, cursorID string, limit int) (*entity.KnowledgeStructureNodePage, error) {
	view, err := d.structureReadView(ctx, user, domain, viewID)
	if err != nil {
		return nil, err
	}
	if _, err := structureMember(view, source); err != nil {
		return nil, err
	}
	var parentID *string
	if parent != "" {
		node, err := structureReadNode(ctx, user, domain, view, parent)
		if err != nil {
			return nil, err
		}
		if node.SourceID != source {
			return nil, structureReadError(404, "not_found")
		}
		parentID = &parent
	}
	scope := structureReadScope(user, domain, "nodes", viewID, source, parent, limit)
	cursor, err := structureLoadReadCursor(ctx, user, domain, scope, cursorID, limit)
	if err != nil {
		return nil, err
	}
	var rows []entity.KnowledgeStructureNodeRecord
	err = dao.DB.WithContext(ctx).Where("owner_id = ? AND domain_id = ? AND view_id = ? AND source_id = ? AND parent_id = ?", user, domain, viewID, source, parent).
		Order("start_byte ASC, end_byte ASC, kind ASC, id ASC").Offset(cursor.Offset).Limit(limit + 1).Find(&rows).Error
	if err != nil {
		return nil, structureReadNormalize(err)
	}
	page := &entity.KnowledgeStructureNodePage{ViewID: viewID, SourceID: source, ParentID: parentID, Items: []entity.KnowledgeStructureNode{}}
	for _, row := range rows[:min(limit, len(rows))] {
		var node entity.KnowledgeStructureNode
		if json.Unmarshal([]byte(row.Payload), &node) != nil {
			return nil, structureReadError(503, "structure_integrity")
		}
		page.Items = append(page.Items, node)
	}
	if len(rows) > limit {
		cursor.Offset += len(page.Items)
		page.NextCursor, err = structureSaveReadCursor(ctx, user, domain, scope, cursor)
	}
	return page, err
}

// ReadKnowledgeStructureNode reads one fixed byte window from the original
// authority; it never follows a source or view head while paging content.
func (d *DatasetService) ReadKnowledgeStructureNode(ctx context.Context, user, domain, viewID, nodeID string, start, end *int64) (*entity.KnowledgeStructureContent, error) {
	view, err := d.structureReadView(ctx, user, domain, viewID)
	if err != nil {
		return nil, err
	}
	node, err := structureReadNode(ctx, user, domain, view, nodeID)
	if err != nil {
		return nil, err
	}
	if end != nil && start == nil {
		return nil, structureReadError(400, "invalid_request")
	}
	from := node.StartByte
	if start != nil {
		from = *start
	}
	to := node.EndByte
	if end != nil {
		to = *end
	} else if to-from > 65536 {
		to = from + 65536
	}
	if from < node.StartByte || to > node.EndByte || from > to || to-from > 65536 {
		return nil, structureReadError(400, "invalid_request")
	}
	source, err := d.knowledgeSource(ctx, domain, node.SourceRevisionID)
	if err != nil {
		return nil, structureReadNormalize(err)
	}
	body, err := knowledgeSourceBytes(ctx, source)
	if err != nil {
		return nil, structureReadError(503, "source_unavailable")
	}
	if end == nil {
		for to > from && to < int64(len(body)) && !utf8.RuneStart(body[to]) {
			to--
		}
	}
	if !knowledgeRange(body, from, to) {
		return nil, structureReadError(400, "invalid_request")
	}
	var next *int64
	complete := to == node.EndByte
	if !complete {
		next = &to
	}
	return &entity.KnowledgeStructureContent{ViewID: viewID, StructureNodeID: nodeID, SourceNodeID: node.SourceNodeID, SourceID: node.SourceID, SourceRevisionID: node.SourceRevisionID, SourceSHA256: source.SHA256, FragmentSHA256: directorySHA(body[from:to]), CoordinateSpace: "source_utf8", StartByte: from, EndByte: to, Text: string(body[from:to]), Complete: complete, NextStartByte: next}, nil
}

func structureContains(view string, node entity.KnowledgeStructureNodeRecord) (entity.KnowledgeStructureRelation, error) {
	target := node.ID
	evidence := entity.KnowledgeStructureEvidence{SourceID: node.SourceID, SourceRevisionID: node.SourceRevisionID, StartByte: node.StartByte, EndByte: node.EndByte, FragmentSHA256: node.FragmentSHA256}
	id, err := entity.KnowledgeLifecycleDigest(map[string]any{"kind": "knowledge-structure-relation/0.1.0", "view_id": view, "relation_kind": "contains", "from_node_id": node.ParentID, "to_node_id": target, "reference": "", "evidence": evidence})
	return entity.KnowledgeStructureRelation{RelationID: id, ViewID: view, Kind: "contains", FromNodeID: node.ParentID, ToNodeID: &target, Resolution: "resolved", Reason: "syntax_containment", Reference: "", Evidence: evidence}, err
}

func (d *DatasetService) ListKnowledgeStructureRelations(ctx context.Context, user, domain, viewID, nodeID, direction, kind, cursorID string, limit int) (*entity.KnowledgeStructureRelationPage, error) {
	view, err := d.structureReadView(ctx, user, domain, viewID)
	if err != nil {
		return nil, err
	}
	if _, err = structureReadNode(ctx, user, domain, view, nodeID); err != nil {
		return nil, err
	}
	if direction == "" {
		direction = "both"
	}
	if !slices.Contains([]string{"in", "out", "both"}, direction) || kind != "" && !slices.Contains([]string{"contains", "imports_file", "exports_symbol", "references_type", "calls_local"}, kind) {
		return nil, structureReadError(400, "invalid_request")
	}
	scope := structureReadScope(user, domain, "relations", viewID, nodeID, direction, kind, limit)
	cursor, err := structureLoadReadCursor(ctx, user, domain, scope, cursorID, limit)
	if err != nil {
		return nil, err
	}
	items := []entity.KnowledgeStructureRelation{}
	if kind == "" || kind != "contains" {
		query := dao.DB.WithContext(ctx).Where("owner_id = ? AND domain_id = ? AND view_id = ? AND id > ?", user, domain, viewID, cursor.LastID)
		switch direction {
		case "in":
			query = query.Where("to_node_id = ?", nodeID)
		case "out":
			query = query.Where("from_node_id = ?", nodeID)
		default:
			query = query.Where("from_node_id = ? OR to_node_id = ?", nodeID, nodeID)
		}
		if kind != "" {
			query = query.Where("kind = ?", kind)
		}
		var rows []entity.KnowledgeStructureRelationRecord
		if err = query.Order("id ASC").Limit(limit + 1).Find(&rows).Error; err != nil {
			return nil, structureReadNormalize(err)
		}
		for _, row := range rows {
			var rel entity.KnowledgeStructureRelation
			if json.Unmarshal([]byte(row.Payload), &rel) != nil {
				return nil, structureReadError(503, "structure_integrity")
			}
			items = append(items, rel)
		}
	}
	if kind == "" || kind == "contains" {
		query := dao.DB.WithContext(ctx).Where("owner_id = ? AND domain_id = ? AND view_id = ? AND parent_id <> ?", user, domain, viewID, "")
		switch direction {
		case "in":
			query = query.Where("id = ?", nodeID)
		case "out":
			query = query.Where("parent_id = ?", nodeID)
		default:
			query = query.Where("id = ? OR parent_id = ?", nodeID, nodeID)
		}
		var rows []entity.KnowledgeStructureNodeRecord
		if err = query.Select("id, source_id, source_revision_id, parent_id, start_byte, end_byte, fragment_sha256").Limit(8193).Find(&rows).Error; err != nil {
			return nil, structureReadNormalize(err)
		}
		if len(rows) > 8192 {
			return nil, structureReadError(503, "structure_integrity")
		}
		for _, row := range rows {
			rel, e := structureContains(viewID, row)
			if e != nil {
				return nil, structureReadNormalize(e)
			}
			if rel.RelationID > cursor.LastID {
				items = append(items, rel)
			}
		}
	}
	slices.SortFunc(items, func(a, b entity.KnowledgeStructureRelation) int { return strings.Compare(a.RelationID, b.RelationID) })
	page := &entity.KnowledgeStructureRelationPage{ViewID: viewID, NodeID: nodeID, Direction: direction, Items: items[:min(limit, len(items))]}
	if kind != "" {
		page.Kind = &kind
	}
	if len(items) > limit {
		cursor.LastID = page.Items[len(page.Items)-1].RelationID
		page.NextCursor, err = structureSaveReadCursor(ctx, user, domain, scope, cursor)
	}
	return page, err
}

func (d *DatasetService) ListKnowledgeStructureTasks(ctx context.Context, user, domain, project, cursorID string, limit int) (*entity.KnowledgeStructureTaskPage, error) {
	if err := d.AuthorizeKnowledgeStructure(ctx, user, domain); err != nil {
		return nil, err
	}
	if err := knowledgeProject(dao.DB.WithContext(ctx), domain, project); err != nil {
		return nil, structureReadNormalize(err)
	}
	scope := structureReadScope(user, domain, "tasks", project, limit)
	cursor, err := structureLoadReadCursor(ctx, user, domain, scope, cursorID, limit)
	if err != nil {
		return nil, err
	}
	query := dao.DB.WithContext(ctx).Where("owner_id = ? AND domain_id = ? AND project_id = ? AND protocol IN ?", user, domain, project, []string{"", "code_structure/0.1.0"})
	if cursorID == "" {
		var first entity.KnowledgeStructureTaskRecord
		e := query.Order("created_at DESC, id DESC").Take(&first).Error
		if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
			return nil, structureReadNormalize(e)
		}
		if e == nil {
			cursor.UpperTime, cursor.UpperID = first.CreatedAt, first.ID
		}
	}
	page := &entity.KnowledgeStructureTaskPage{DomainID: domain, ProjectID: project, CapturedAt: cursor.CapturedAt.Format(time.RFC3339Nano), Items: []entity.KnowledgeStructureTask{}}
	if cursor.UpperID == "" {
		return page, nil
	}
	query = query.Where("created_at < ? OR (created_at = ? AND id <= ?)", cursor.UpperTime, cursor.UpperTime, cursor.UpperID)
	var rows []entity.KnowledgeStructureTaskRecord
	err = query.Order("created_at DESC, id DESC").Offset(cursor.Offset).Limit(limit + 1).Find(&rows).Error
	if err != nil {
		return nil, structureReadNormalize(err)
	}
	for _, row := range rows[:min(limit, len(rows))] {
		var task entity.KnowledgeStructureTask
		if json.Unmarshal([]byte(row.Payload), &task) != nil {
			return nil, structureReadError(503, "structure_integrity")
		}
		page.Items = append(page.Items, task)
		raw, e := json.Marshal(page)
		if e != nil {
			return nil, structureReadNormalize(e)
		}
		if len(raw) > 261000 {
			page.Items = page.Items[:len(page.Items)-1]
			break
		}
	}
	if len(rows) > len(page.Items) {
		if len(page.Items) == 0 {
			return nil, structureReadError(503, "structure_integrity")
		}
		cursor.Offset += len(page.Items)
		page.NextCursor, err = structureSaveReadCursor(ctx, user, domain, scope, cursor)
	}
	return page, err
}

func (d *DatasetService) GetKnowledgeStructureHead(ctx context.Context, user, domain, project string) (*entity.KnowledgeStructureHead, error) {
	if err := d.AuthorizeKnowledgeStructure(ctx, user, domain); err != nil {
		return nil, err
	}
	if err := knowledgeProject(dao.DB.WithContext(ctx), domain, project); err != nil {
		return nil, structureReadNormalize(err)
	}
	var stored entity.KnowledgeStructureHeadRecord
	err := dao.DB.WithContext(ctx).Where("project_id = ? AND domain_id = ? AND owner_id = ?", project, domain, user).Take(&stored).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, structureReadNormalize(err)
	}
	var view *entity.KnowledgeStructureView
	if err == nil {
		view, err = d.structureReadView(ctx, user, domain, stored.ViewID)
		if err != nil {
			return nil, err
		}
	}
	head := &entity.KnowledgeStructureHead{DomainID: domain, ProjectID: project, Currentness: "none", Reasons: []string{}, AffectedSources: []entity.KnowledgeStructureHeadIssue{}}
	err = dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		kb, e := lockKnowledgeImportDomain(ctx, tx, user, domain)
		if e != nil {
			return e
		}
		var currentPointer entity.KnowledgeStructureHeadRecord
		pointerErr := tx.Where("project_id = ? AND domain_id = ? AND owner_id = ?", project, domain, user).Take(&currentPointer).Error
		if pointerErr != nil && !errors.Is(pointerErr, gorm.ErrRecordNotFound) {
			return pointerErr
		}
		if (view == nil && pointerErr == nil) || (view != nil && (pointerErr != nil || currentPointer.ViewID != view.ViewID)) {
			return structureReadError(503, "unavailable")
		}
		manifest, e := projectManifestHead(tx, domain, project)
		if e != nil {
			return e
		}
		head.CurrentManifestID = manifest
		head.CheckedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if view == nil {
			return nil
		}
		head.ViewID = &view.ViewID
		head.Currentness = "current"
		var policy entity.KnowledgeStructurePolicyRecord
		e = tx.Where("domain_id = ? AND owner_id = ?", domain, user).Take(&policy).Error
		if errors.Is(e, gorm.ErrRecordNotFound) {
			policy.Version = 1
		} else if e != nil {
			return e
		}
		global := []string{}
		if manifest == nil || *manifest != view.ManifestID {
			global = append(global, "manifest_changed")
		}
		if kb.KnowledgePolicyVersion != view.FoundationPolicyVersion || policy.Version != view.StructurePolicyVersion {
			global = append(global, "policy_changed")
		}
		for _, member := range view.Members {
			reasons := slices.Clone(global)
			current, e := lifecycleHead(tx, domain, member.SourceID)
			if e != nil {
				return e
			}
			state, e := lifecycleState(tx, domain, member.SourceID)
			if e != nil {
				return e
			}
			if current.ID != member.SourceRevisionID || current.SHA256 != member.SHA256 {
				reasons = append(reasons, "source_changed")
			}
			if state.Version != member.LifecycleVersion {
				reasons = append(reasons, "lifecycle_changed")
			}
			if state.State == "archived" {
				reasons = append(reasons, "source_archived")
			}
			if current.RelativePath != member.RelativePath {
				reasons = append(reasons, "position_changed")
			}
			class := entity.KnowledgeStructureClassification{GroupID: current.GroupID, ComponentSide: current.ComponentSide, Language: current.Language}
			if directoryJSON(class) != directoryJSON(member.Classification) {
				reasons = append(reasons, "classification_changed")
			}
			if len(reasons) > 0 {
				head.AffectedSources = append(head.AffectedSources, entity.KnowledgeStructureHeadIssue{SourceID: member.SourceID, Reasons: reasons})
				for _, reason := range reasons {
					if !slices.Contains(head.Reasons, reason) {
						head.Reasons = append(head.Reasons, reason)
					}
				}
			}
		}
		if len(head.Reasons) > 0 {
			head.Currentness = "stale"
		}
		return nil
	})
	return head, structureReadNormalize(err)
}
