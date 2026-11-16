package dataset

import (
	"context"
	"errors"
	"slices"
	"sort"
	"time"

	"gorm.io/gorm"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func accessTenantOwner(ctx context.Context, tx *gorm.DB, actor, tenant string) error {
	if err := accessTenant(ctx, tx, actor, tenant, true); err != nil {
		return err
	}
	var row entity.UserTenant
	return accessLocked(tx, true).Where("user_id = ? AND tenant_id = ? AND status = ? AND role = ?", actor, tenant, "1", "owner").Take(&row).Error
}

func accessLiveMember(tx *gorm.DB, user, tenant string) error {
	var row entity.User
	if err := accessLocked(tx, true).Select("id").Where("id = ? AND status = ? AND is_active = ? AND is_authenticated = ? AND is_anonymous = ?", user, "1", "1", "1", "0").Take(&row).Error; err != nil {
		return err
	}
	return accessMembership(tx, user, tenant, true)
}

func accessDepartment(tx *gorm.DB, tenant, id string) (*entity.KnowledgeAccessDepartmentRecord, error) {
	var row entity.KnowledgeAccessDepartmentRecord
	if err := accessLocked(tx, true).Where("id = ? AND tenant_id = ?", id, tenant).Take(&row).Error; err != nil {
		return nil, err
	}
	if row.Version < 1 || row.Version > entity.KnowledgeAccessMaxGeneration {
		return nil, accessError(409, "integrity_error")
	}
	return &row, nil
}

func accessDepartmentDTO(tx *gorm.DB, row *entity.KnowledgeAccessDepartmentRecord, roster bool) (*entity.KnowledgeAccessDepartment, error) {
	data := &entity.KnowledgeAccessDepartment{DepartmentID: row.ID, TenantID: row.TenantID, Name: row.Name, Version: row.Version, Enabled: row.Enabled}
	if !roster {
		return data, nil
	}
	var members []entity.KnowledgeAccessDepartmentMemberRecord
	if err := accessLocked(tx, true).Where("department_id = ?", row.ID).Order("user_id ASC").Limit(257).Find(&members).Error; err != nil {
		return nil, err
	}
	if len(members) > 256 {
		return nil, accessError(413, "capacity_exceeded")
	}
	data.UserIDs = []string{}
	for _, member := range members {
		if !isKnowledgeID(member.UserID, 32) || slices.Contains(data.UserIDs, member.UserID) {
			return nil, accessError(409, "integrity_error")
		}
		data.UserIDs = append(data.UserIDs, member.UserID)
	}
	data.MemberCount = accessPtr(int64(len(data.UserIDs)))
	return data, nil
}

func accessSetDepartment(tx *gorm.DB, actor string, now time.Time, req *entity.KnowledgeAccessSetDepartmentRequest) (*entity.KnowledgeAccessDepartment, *entity.KnowledgeAccessDepartment, bool, error) {
	if req.DepartmentID == nil && req.ExpectedVersion != 0 || req.DepartmentID != nil && req.ExpectedVersion < 1 {
		return nil, nil, false, accessError(400, "invalid_request")
	}
	ids := append([]string{}, req.UserIDs...)
	sort.Strings(ids)
	for _, id := range ids {
		if err := accessLiveMember(tx, id, req.TenantID); err != nil {
			return nil, nil, false, err
		}
	}
	var row entity.KnowledgeAccessDepartmentRecord
	var before *entity.KnowledgeAccessDepartment
	if req.DepartmentID == nil {
		var count int64
		if err := tx.Model(&row).Where("tenant_id = ?", req.TenantID).Count(&count).Error; err != nil {
			return nil, nil, false, err
		}
		if count >= 64 {
			return nil, nil, false, accessError(413, "capacity_exceeded")
		}
		row = entity.KnowledgeAccessDepartmentRecord{ID: common.GenerateUUID(), TenantID: req.TenantID, Version: 1, CreatedBy: actor, CreatedAt: now}
	} else {
		old, err := accessDepartment(tx, req.TenantID, *req.DepartmentID)
		if err != nil {
			return nil, nil, false, err
		}
		if old.Version != req.ExpectedVersion {
			return nil, nil, false, accessError(409, "version_conflict")
		}
		before, err = accessDepartmentDTO(tx, old, true)
		if err != nil {
			return nil, nil, false, err
		}
		if old.Name == req.Name && old.Enabled == req.Enabled && slices.Equal(before.UserIDs, ids) {
			return before, before, false, nil
		}
		if old.Version >= entity.KnowledgeAccessMaxGeneration {
			return nil, nil, false, accessError(503, "unavailable")
		}
		row = *old
		row.Version++
	}
	row.Name, row.Enabled, row.UpdatedAt = req.Name, req.Enabled, now
	if req.DepartmentID == nil {
		if err := tx.Create(&row).Error; err != nil {
			return nil, nil, false, err
		}
	} else {
		result := tx.Model(&row).Where("id = ? AND tenant_id = ? AND version = ?", row.ID, row.TenantID, req.ExpectedVersion).Updates(map[string]any{"name": row.Name, "enabled": row.Enabled, "version": row.Version, "updated_at": now})
		if result.Error != nil {
			return nil, nil, false, result.Error
		}
		if result.RowsAffected != 1 {
			return nil, nil, false, accessError(409, "version_conflict")
		}
		if err := tx.Where("department_id = ?", row.ID).Delete(&entity.KnowledgeAccessDepartmentMemberRecord{}).Error; err != nil {
			return nil, nil, false, err
		}
	}
	for _, id := range ids {
		member := entity.KnowledgeAccessDepartmentMemberRecord{ID: common.GenerateUUID(), DepartmentID: row.ID, UserID: id, CreatedAt: now}
		if err := tx.Create(&member).Error; err != nil {
			return nil, nil, false, err
		}
	}
	data := &entity.KnowledgeAccessDepartment{DepartmentID: row.ID, TenantID: row.TenantID, Name: row.Name, Version: row.Version, Enabled: row.Enabled, MemberCount: accessPtr(int64(len(ids))), UserIDs: ids}
	return data, before, true, nil
}

func (d *DatasetService) accessListDepartments(ctx context.Context, actor, requestID string, req entity.KnowledgeAccessListDepartmentsRequest) (int, []byte, error) {
	var encoded []byte
	err := dao.KnowledgeAccessTransaction(ctx, accessDB(ctx), func(tx *gorm.DB) error {
		if _, err := dao.LockKnowledgeAccessTenants(tx, []string{req.TenantID}, true); err != nil {
			return err
		}
		if err := accessTenant(ctx, tx, actor, req.TenantID, true); err != nil {
			return err
		}
		var membership entity.UserTenant
		if err := accessLocked(tx, true).Where("user_id = ? AND tenant_id = ? AND status = ?", actor, req.TenantID, "1").Take(&membership).Error; err != nil {
			return err
		}
		owner := membership.Role == "owner"
		query := tx.Where("tenant_id = ?", req.TenantID)
		if !owner {
			query = query.Where("enabled = ?", true)
		}
		var rows []entity.KnowledgeAccessDepartmentRecord
		if err := query.Order("id ASC").Limit(65).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) > 64 {
			return accessError(413, "capacity_exceeded")
		}
		data := entity.KnowledgeAccessListDepartmentsData{Items: []entity.KnowledgeAccessDepartment{}, Pagination: "none"}
		for _, row := range rows {
			department, err := accessDepartmentDTO(tx, &row, owner)
			if err != nil {
				return err
			}
			data.Items = append(data.Items, *department)
		}
		var err error
		encoded, err = accessEncode("listDepartments", requestID, data)
		if err != nil {
			return err
		}
		now, err := accessNow(tx)
		if err != nil {
			return err
		}
		return tx.Create(&entity.KnowledgeAccessAuditRecord{ID: common.GenerateUUID(), TenantID: req.TenantID, ActorUserID: &actor, Action: "listDepartments", Outcome: "allowed", RequestID: requestID, OccurredAt: now}).Error
	})
	return 200, encoded, err
}

func accessGrantSubject(tx *gorm.DB, grant entity.KnowledgeAccessGrantRecord, actor string) (bool, error) {
	switch grant.SubjectKind {
	case "user":
		return grant.SubjectID == actor, nil
	case "department":
		department, err := accessDepartment(tx, grant.TenantID, grant.SubjectID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if !department.Enabled {
			return false, nil
		}
		var rows []entity.KnowledgeAccessDepartmentMemberRecord
		if err = accessLocked(tx, true).Where("department_id = ? AND user_id = ?", department.ID, actor).Limit(2).Find(&rows).Error; err != nil {
			return false, err
		}
		if len(rows) > 1 {
			return false, accessError(409, "integrity_error")
		}
		return len(rows) == 1, nil
	}
	return false, accessError(409, "integrity_error")
}
