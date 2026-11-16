package dataset

import (
	"context"
	"errors"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	structure "ragflow/internal/parser/structure"
)

var errStructureLeaseLost = errors.New("structure lease lost")
var errStructureStopped = errors.New("structure task stopped")

type structureClaim struct {
	Protocol string
	DomainID string
	OwnerID  string
	TaskID   string
	RunnerID string
	Epoch    int64
	Deadline time.Time
}

// structureInternalTransaction may retire an existing explicit task after
// revocation. Admission and publication separately require the current owner.
func structureInternalTransaction(ctx context.Context, domain string, fn func(*gorm.DB) error) error {
	return dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := dao.LockKnowledgeAccessDomain(ctx, tx, domain); err != nil {
			return err
		}
		var kb entity.Knowledgebase
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", domain).Take(&kb).Error; err != nil {
			return err
		}
		return fn(tx)
	})
}

func structureLoadLease(tx *gorm.DB, claim *structureClaim) (*entity.KnowledgeStructureLeaseRecord, *entity.KnowledgeStructureTaskRecord, *entity.KnowledgeStructureTask, time.Time, error) {
	lease, _, now, err := structureOwnedLease(tx, claim)
	if err != nil {
		return nil, nil, nil, now, err
	}
	row, task, err := structureLoadTask(tx, claim.OwnerID, claim.DomainID, claim.TaskID)
	if err != nil {
		return nil, nil, nil, now, err
	}
	return lease, row, task, now, nil
}

func structureReleaseLease(tx *gorm.DB, claim *structureClaim, now time.Time) error {
	return tx.Model(&entity.KnowledgeStructureLeaseRecord{}).Where("domain_id = ? AND task_id = ? AND runner_id = ? AND epoch = ?", claim.DomainID, claim.TaskID, claim.RunnerID, claim.Epoch).Updates(map[string]any{"task_id": "", "runner_id": "", "expires_at": now, "updated_at": now}).Error
}

func structureFinish(tx *gorm.DB, row *entity.KnowledgeStructureTaskRecord, task *entity.KnowledgeStructureTask, state, code, failedSource string, now time.Time) error {
	if structureTerminal(task.State) {
		return nil
	}
	task.State = state
	task.ResultViewID = nil
	task.FailureCode = nil
	if state == "failed" {
		task.FailureCode = &code
	}
	for i := range task.Members {
		member := &task.Members[i]
		if member.SourceID == failedSource && state == "failed" {
			member.State = "failed"
			member.Diagnostics = []entity.KnowledgeStructureDiagnostic{{Code: code, Message: code}}
		} else if member.State == "pending" || member.State == "processing" || state == "cancelled" && (member.State == "staged" || member.State == "reused") {
			member.State = "cancelled"
		}
	}
	return structureSaveTask(tx, row, task, now)
}

func structureCapturedEqual(a, b *structureInputs) bool {
	left, err := structureJSON(a)
	if err != nil {
		return false
	}
	right, err := structureJSON(b)
	return err == nil && left == right
}

func structureCheckTaskInputs(tx *gorm.DB, kb *entity.Knowledgebase, row *entity.KnowledgeStructureTaskRecord, task *entity.KnowledgeStructureTask) (*structureInputs, string, error) {
	inputs, err := structureDecode[structureInputs](row.Inputs)
	if err != nil {
		return nil, "", err
	}
	digest, err := structureTaskDigest(row.OwnerID, row.DomainID, task.OriginalRequest)
	if err != nil || digest != row.RequestSHA256 {
		return nil, "", structureError(409, "structure_integrity")
	}
	producer, err := entity.KnowledgeStructureProducerDigest(task.OriginalRequest.Aliases)
	if err != nil || producer != row.ProducerSHA256 {
		return nil, "", structureError(409, "structure_integrity")
	}
	current, reason, err := structureCapture(tx, kb, task.OriginalRequest)
	if err != nil || reason != "" {
		return nil, reason, err
	}
	if !structureCapturedEqual(inputs, current) {
		return nil, "revision_changed", nil
	}
	return inputs, "", nil
}

// structureCheckpoint checks the original policy/source/head fences before
// progressing or extending a lease. A cancellation is retired at a checkpoint.
func (d *DatasetService) structureCheckpoint(ctx context.Context, claim *structureClaim, renew bool, edit func(*gorm.DB, *entity.KnowledgeStructureTaskRecord, *entity.KnowledgeStructureTask, *structureInputs, time.Time) error) error {
	_, authErr := d.knowledgeBase(ctx, claim.OwnerID, claim.DomainID, true)
	permissionRevoked := false
	if authErr != nil {
		var denied *StructureOperationError
		if normalized := structureNormalize(authErr); errors.As(normalized, &denied) && denied.Status == 404 {
			permissionRevoked = true
		} else {
			return authErr
		}
	}
	stopped := false
	err := structureInternalTransaction(ctx, claim.DomainID, func(tx *gorm.DB) error {
		lease, row, task, now, err := structureLoadLease(tx, claim)
		if err != nil {
			return err
		}
		stop := func(state, code string) error {
			stopped = true
			if err := structureFinish(tx, row, task, state, code, "", now); err != nil {
				return err
			}
			return structureReleaseLease(tx, claim, now)
		}
		if task.State == "cancel_requested" {
			return stop("cancelled", "")
		}
		if !now.Before(row.DeadlineAt) {
			return stop("failed", "execution_expired")
		}
		if permissionRevoked {
			return stop("failed", "permission_revoked")
		}
		kb, err := lockKnowledgeImportDomain(ctx, tx, claim.OwnerID, claim.DomainID)
		if err != nil {
			var public *KnowledgeError
			if errors.As(err, &public) && public.Status == 404 || errors.Is(err, gorm.ErrRecordNotFound) {
				return stop("failed", "permission_revoked")
			}
			return err
		}
		inputs, reason, err := structureCheckTaskInputs(tx, kb, row, task)
		if err != nil {
			var public *StructureOperationError
			if errors.As(err, &public) && public.Code == "structure_integrity" {
				return stop("failed", "structure_integrity")
			}
			var sourceErr *KnowledgeError
			if errors.Is(err, gorm.ErrRecordNotFound) || errors.As(err, &sourceErr) && sourceErr.Status == 404 {
				return stop("failed", "source_unavailable")
			}
			if sourceErr != nil && sourceErr.Code == "lifecycle_integrity" {
				return stop("failed", "lifecycle_integrity")
			}
			return err
		}
		if reason != "" {
			return stop("failed", reason)
		}
		if renew {
			if err := structureRenewClaimLease(tx, claim, lease, now, row.DeadlineAt); err != nil {
				return err
			}
		}
		if edit != nil {
			return edit(tx, row, task, inputs, now)
		}
		return nil
	})
	if err == nil && stopped {
		return errStructureStopped
	}
	return err
}

type structureArtifact struct {
	SelectionSHA256 string         `json:"selection_sha256"`
	File            structure.File `json:"file"`
}

func structureArtifactID(owner, domain, producer string, member entity.KnowledgeStructureViewMember) string {
	id, _ := entity.KnowledgeLifecycleDigest(map[string]any{"kind": "knowledge-structure-artifact/0.1.0", "owner_id": owner, "domain_id": domain, "source_id": member.SourceID, "content_sha256": member.SHA256, "parser": member.Parser, "producer_sha256": producer})
	return id
}

func structureSelectionDigest(inputs *structureInputs) (string, error) {
	members := make([]map[string]any, 0, len(inputs.Members))
	for _, member := range inputs.Members {
		members = append(members, map[string]any{"source_id": member.SourceID, "content_sha256": member.SHA256, "parser": member.Parser, "relative_path": member.RelativePath})
	}
	return entity.KnowledgeLifecycleDigest(map[string]any{"kind": "knowledge-structure-cache-selection/0.1.0", "members": members})
}

func structureCachedBuild(ctx context.Context, owner, domain, producer string, inputs *structureInputs) (*structure.Output, error) {
	selection, err := structureSelectionDigest(inputs)
	if err != nil {
		return nil, err
	}
	revisions := map[string]string{}
	for _, member := range inputs.Members {
		revisions[member.SourceID] = member.SourceRevisionID
	}
	output := &structure.Output{Files: []structure.File{}}
	for _, member := range inputs.Members {
		var row entity.KnowledgeStructureArtifactRecord
		err := dao.DB.WithContext(ctx).Where("id = ? AND owner_id = ? AND domain_id = ? AND source_id = ? AND content_sha256 = ? AND producer_sha256 = ? AND parser = ?", structureArtifactID(owner, domain, producer, member), owner, domain, member.SourceID, member.SHA256, producer, member.Parser).Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		cached, err := structureDecode[structureArtifact](row.Payload)
		if err != nil || cached.SelectionSHA256 != selection || cached.File.SourceID != member.SourceID || cached.File.SHA256 != member.SHA256 || cached.File.Path != member.RelativePath || cached.File.Parser != member.Parser || int64(cached.File.ByteLength) != member.ByteLength {
			return nil, nil
		}
		cached.File.RevisionID = member.SourceRevisionID
		for i := range cached.File.Relations {
			relation := &cached.File.Relations[i]
			revision, ok := revisions[relation.Evidence.SourceID]
			if !ok {
				return nil, nil
			}
			relation.Evidence.RevisionID = revision
		}
		output.Files = append(output.Files, cached.File)
	}
	return output, nil
}

func structureSaveArtifact(tx *gorm.DB, owner, domain, producer, selection string, member entity.KnowledgeStructureViewMember, file structure.File, now time.Time) error {
	payload, err := structureJSON(structureArtifact{SelectionSHA256: selection, File: file})
	if err != nil {
		return err
	}
	row := entity.KnowledgeStructureArtifactRecord{ID: structureArtifactID(owner, domain, producer, member), OwnerID: owner, DomainID: domain, SourceID: member.SourceID, ContentSHA256: member.SHA256, ProducerSHA256: producer, Parser: member.Parser, Payload: payload, CreatedAt: now}
	return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: clause.AssignmentColumns([]string{"payload", "created_at"})}).Create(&row).Error
}

func (d *DatasetService) structureFailClaim(ctx context.Context, claim *structureClaim, code, source string) {
	work, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = structureInternalTransaction(work, claim.DomainID, func(tx *gorm.DB) error {
		_, row, task, now, err := structureLoadLease(tx, claim)
		if err != nil {
			return err
		}
		state := "failed"
		if task.State == "cancel_requested" {
			state, code = "cancelled", ""
		}
		if err := structureFinish(tx, row, task, state, code, source, now); err != nil {
			return err
		}
		return structureReleaseLease(tx, claim, now)
	})
}

func (d *DatasetService) runKnowledgeStructure(ctx context.Context, claim *structureClaim) {
	work, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	defer func() {
		if ctx.Err() == nil && work.Err() != nil {
			d.structureFailClaim(ctx, claim, "execution_expired", "")
		}
	}()
	heartbeatDone := make(chan struct{})
	defer close(heartbeatDone)
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-work.Done():
				return
			case <-heartbeatDone:
				return
			case <-ticker.C:
				beat, stop := context.WithTimeout(work, 5*time.Second)
				err := d.structureCheckpoint(beat, claim, true, nil)
				stop()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()
	// A lightweight cancellation watch cancels the parser context without
	// extending the durable lease or inventing task progress revisions.
	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-work.Done():
				return
			case <-heartbeatDone:
				return
			case <-ticker.C:
				var state struct{ State string }
				if err := dao.DB.WithContext(work).Model(&entity.KnowledgeStructureTaskRecord{}).Select("state").Where("id = ? AND owner_id = ? AND domain_id = ?", claim.TaskID, claim.OwnerID, claim.DomainID).Take(&state).Error; err == nil && state.State == "cancel_requested" {
					cancel()
					return
				}
			}
		}
	}()
	var fixed *structureInputs
	var taskSnapshot *entity.KnowledgeStructureTask
	err := d.structureCheckpoint(work, claim, false, func(_ *gorm.DB, _ *entity.KnowledgeStructureTaskRecord, task *entity.KnowledgeStructureTask, inputs *structureInputs, _ time.Time) error {
		fixed, taskSnapshot = inputs, task
		return nil
	})
	if err != nil {
		return
	}
	content := make(map[string][]byte, len(fixed.Members))
	producerInputs := make([]structure.Input, 0, len(fixed.Members))
	for _, member := range fixed.Members {
		err := d.structureCheckpoint(work, claim, false, func(tx *gorm.DB, row *entity.KnowledgeStructureTaskRecord, task *entity.KnowledgeStructureTask, _ *structureInputs, now time.Time) error {
			for i := range task.Members {
				if task.Members[i].SourceID == member.SourceID && task.Members[i].State == "pending" {
					task.Members[i].State = "processing"
					return structureSaveTask(tx, row, task, now)
				}
			}
			return nil
		})
		if err != nil {
			return
		}
		source, err := d.GetKnowledgeSource(work, claim.OwnerID, claim.DomainID, member.SourceRevisionID)
		if err != nil {
			d.structureFailClaim(ctx, claim, "source_unavailable", member.SourceID)
			return
		}
		raw, err := knowledgeSourceBytes(work, source)
		if err != nil || !utf8.Valid(raw) || int64(len(raw)) != member.ByteLength || directorySHA(raw) != member.SHA256 {
			d.structureFailClaim(ctx, claim, "source_unavailable", member.SourceID)
			return
		}
		content[member.SourceID] = raw
		producerInputs = append(producerInputs, structure.Input{SourceID: member.SourceID, RevisionID: member.SourceRevisionID, Path: member.RelativePath, Parser: member.Parser, Content: raw})
	}
	output, err := structureCachedBuild(work, claim.OwnerID, claim.DomainID, taskSnapshot.ProducerSHA256, fixed)
	if err != nil {
		// A cache failure is not permission to publish an unverified result.
		return
	}
	reused := output != nil
	if output == nil {
		aliases := make([]structure.Alias, 0, len(fixed.Aliases))
		for _, alias := range fixed.Aliases {
			aliases = append(aliases, structure.Alias{Prefix: alias.Prefix, Target: alias.Target})
		}
		built, buildErr := structure.Build(work, producerInputs, aliases)
		if buildErr != nil {
			if ctx.Err() == nil {
				code, failedSource := "parse_error", ""
				var controlled *structure.Error
				if errors.As(buildErr, &controlled) && structureTextBounded(controlled.Code, 1, 64) {
					code, failedSource = controlled.Code, controlled.SourceID
				}
				if work.Err() != nil {
					code = "execution_expired"
				}
				d.structureFailClaim(ctx, claim, code, failedSource)
			}
			return
		}
		output = &built
	}
	bound, err := structureBindBuild(claim.OwnerID, claim.DomainID, taskSnapshot.ProjectID, taskSnapshot.ProducerSHA256, fixed, *output, content)
	if err != nil {
		d.structureFailClaim(ctx, claim, "structure_integrity", "")
		return
	}
	selection, err := structureSelectionDigest(fixed)
	if err != nil {
		d.structureFailClaim(ctx, claim, "structure_integrity", "")
		return
	}
	for _, file := range output.Files {
		err := d.structureCheckpoint(work, claim, false, func(tx *gorm.DB, row *entity.KnowledgeStructureTaskRecord, task *entity.KnowledgeStructureTask, inputs *structureInputs, now time.Time) error {
			var member *entity.KnowledgeStructureViewMember
			for i := range inputs.Members {
				if inputs.Members[i].SourceID == file.SourceID {
					member = &inputs.Members[i]
					break
				}
			}
			if member == nil {
				return structureError(409, "structure_integrity")
			}
			if err := structureSaveArtifact(tx, claim.OwnerID, claim.DomainID, task.ProducerSHA256, selection, *member, file, now); err != nil {
				return err
			}
			for i := range task.Members {
				if task.Members[i].SourceID == file.SourceID {
					task.Members[i].State = "staged"
					if reused {
						task.Members[i].State = "reused"
					}
					task.Members[i].NodeCount = bound.MemberNodes[file.SourceID]
					task.Members[i].RelationCount = bound.MemberRelations[file.SourceID]
					task.Members[i].Diagnostics = bound.Diagnostics[file.SourceID]
					return structureSaveTask(tx, row, task, now)
				}
			}
			return structureError(409, "structure_integrity")
		})
		if err != nil {
			return
		}
	}
	err = d.structurePublish(work, claim, bound)
	if err != nil && ctx.Err() == nil && !errors.Is(err, errStructureStopped) && !errors.Is(err, errStructureLeaseLost) {
		// A rolled-back activation never changes the old visible view.
		d.structureFailClaim(ctx, claim, "publication_failed", "")
	}
}

func (d *DatasetService) structurePublish(ctx context.Context, claim *structureClaim, bound *structureBoundBuild) error {
	return d.structureCheckpoint(ctx, claim, false, func(tx *gorm.DB, row *entity.KnowledgeStructureTaskRecord, task *entity.KnowledgeStructureTask, _ *structureInputs, now time.Time) error {
		view := bound.View
		view.PublishedAt = structureTimestamp(now)
		var existing entity.KnowledgeStructureViewRecord
		err := tx.Where("id = ?", view.ViewID).Take(&existing).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		alreadyPublished := err == nil
		if alreadyPublished {
			if existing.OwnerID != claim.OwnerID || existing.DomainID != claim.DomainID || existing.ProjectID != task.ProjectID || !existing.Published {
				return structureError(409, "structure_integrity")
			}
		} else {
			payload, err := structureJSON(view)
			if err != nil {
				return err
			}
			if err := tx.Create(&entity.KnowledgeStructureViewRecord{ID: view.ViewID, OwnerID: claim.OwnerID, DomainID: claim.DomainID, ProjectID: task.ProjectID, TaskID: task.TaskID, Published: true, Payload: payload, CreatedAt: now, PublishedAt: now}).Error; err != nil {
				return err
			}
			if len(bound.Nodes) != 0 {
				if err := tx.CreateInBatches(&bound.Nodes, 128).Error; err != nil {
					return err
				}
			}
			if len(bound.Relations) != 0 {
				if err := tx.CreateInBatches(&bound.Relations, 128).Error; err != nil {
					return err
				}
			}
		}
		// Verify the lease again after bulk inserts. Expiration/deadline cannot
		// be hidden by holding the domain row lock during a long publication.
		_, _, _, finalNow, err := structureLoadLease(tx, claim)
		if err != nil {
			return err
		}
		if !finalNow.Before(row.DeadlineAt) {
			return errStructureLeaseLost
		}
		if err := tx.Save(&entity.KnowledgeStructureHeadRecord{ProjectID: task.ProjectID, DomainID: claim.DomainID, OwnerID: claim.OwnerID, ViewID: view.ViewID, UpdatedAt: finalNow}).Error; err != nil {
			return err
		}
		task.State, task.ResultViewID = "succeeded", &view.ViewID
		task.FailureCode = nil
		for i := range task.Members {
			if alreadyPublished || task.Members[i].State == "reused" {
				task.Members[i].State = "reused"
			} else {
				task.Members[i].State = "published"
			}
		}
		if err := structureSaveTask(tx, row, task, finalNow); err != nil {
			return err
		}
		return structureReleaseLease(tx, claim, finalNow)
	})
}

// StartKnowledgeStructureRuntime resumes only previously admitted explicit
// tasks. It has no ingestion, model, index, directory or network-fetch dispatch.
func StartKnowledgeStructureRuntime(ctx context.Context) {
	if dao.DB == nil {
		return
	}
	d := NewDatasetService()
	slots := make(chan struct{}, 2)
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				var candidates []entity.KnowledgeStructureTaskRecord
				if err := dao.DB.WithContext(ctx).Where("state IN ?", []string{"queued", "running", "cancel_requested"}).Order("created_at ASC, id ASC").Limit(16).Find(&candidates).Error; err != nil {
					continue
				}
				for _, candidate := range candidates {
					select {
					case slots <- struct{}{}:
						go func(record entity.KnowledgeStructureTaskRecord) {
							defer func() { <-slots }()
							claimCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
							claim, err := claimKnowledgeStructure(claimCtx, record, common.GenerateUUID())
							cancel()
							if err == nil && claim != nil {
								switch claim.Protocol {
								case documentTaskProtocol:
									d.runKnowledgeDocument(ctx, claim)
								case entity.KnowledgeParsedProtocol:
									d.runKnowledgeParsed(ctx, claim)
								case "", "code_structure/0.1.0":
									d.runKnowledgeStructure(ctx, claim)
								}
							}
						}(candidate)
					default:
					}
				}
			}
		}
	}()
}
