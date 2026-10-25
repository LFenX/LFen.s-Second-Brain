package dataset

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"io"
	"os"
	"path/filepath"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/utility"
	"runtime"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

type KnowledgeDirectoryError struct {
	Status int
	Code   string
	Reason *string
}

func (e *KnowledgeDirectoryError) Error() string { return e.Code }
func directoryError(status int, code, reason string) error {
	var p *string
	if reason != "" {
		p = &reason
	}
	return &KnowledgeDirectoryError{status, "knowledge_directory_" + code, p}
}
func directoryConflict(reason string) error { return directoryError(409, "conflict", reason) }
func directoryInvalid() error               { return directoryError(400, "invalid", "") }
func directoryMissing() error               { return directoryError(404, "not_found", "") }
func directoryNormalizeError(err error) error {
	if err == nil {
		return nil
	}
	var public *KnowledgeDirectoryError
	if errors.As(err, &public) {
		return err
	}
	var native *KnowledgeError
	if errors.As(err, &native) {
		switch native.Status {
		case 404:
			return directoryMissing()
		case 400, 415:
			return directoryInvalid()
		case 409:
			return directoryConflict("policy_changed")
		}
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return directoryMissing()
	}
	return directoryError(503, "unavailable", "result_unknown")
}
func directoryString(value string) *string { return &value }
func directoryInt(value int64) *int64      { return &value }
func directorySHA(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}
func directoryJSON(value any) string       { raw, _ := json.Marshal(value); return string(raw) }
func directoryTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

type directoryRootConfig struct {
	RootID          string   `json:"root_id"`
	DomainID        string   `json:"domain_id"`
	UserID          string   `json:"user_id"`
	DisplayName     string   `json:"display_name"`
	AbsolutePath    string   `json:"absolute_path"`
	RootVersion     int64    `json:"root_version"`
	ExtraExclusions []string `json:"extra_exclusions"`
}

func directoryRoots() ([]directoryRootConfig, error) {
	name := os.Getenv("LFEN_KNOWLEDGE_ROOTS_FILE")
	if name == "" {
		return []directoryRootConfig{}, nil
	}
	f, err := os.Open(name)
	if err != nil {
		return nil, directoryError(503, "unavailable", "root_unready")
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 262145))
	if err != nil || len(raw) > 262144 || !utf8.Valid(raw) {
		return nil, directoryError(503, "unavailable", "root_unready")
	}
	var config struct {
		Roots []directoryRootConfig `json:"roots"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&config) != nil || decoder.Decode(new(any)) != io.EOF {
		return nil, directoryError(503, "unavailable", "root_unready")
	}
	seen := map[string]bool{}
	counts := map[string]int{}
	for _, r := range config.Roots {
		counts[r.DomainID]++
		if seen[r.RootID] || !isKnowledgeID(r.RootID, 32) || !isKnowledgeID(r.DomainID, 32) || !isKnowledgeID(r.UserID, 32) || r.RootVersion < 1 || !validKnowledgeText(r.DisplayName, 128) || !filepath.IsAbs(r.AbsolutePath) || counts[r.DomainID] > 32 {
			return nil, directoryError(503, "unavailable", "root_unready")
		}
		for _, p := range r.ExtraExclusions {
			if !utility.IsKnowledgeDirectoryPath(p, true) {
				return nil, directoryError(503, "unavailable", "root_unready")
			}
		}
		seen[r.RootID] = true
	}
	return config.Roots, nil
}
func directoryConfig(user, domain, rootID string) (*directoryRootConfig, error) {
	roots, err := directoryRoots()
	if err != nil {
		return nil, err
	}
	for _, r := range roots {
		if r.RootID == rootID && r.DomainID == domain && r.UserID == user {
			return &r, nil
		}
	}
	return nil, directoryMissing()
}
func directoryExcluded(cfg *directoryRootConfig, name string) bool {
	if !utility.IsKnowledgeDirectoryPath(name, true) {
		return true
	}
	for _, prefix := range cfg.ExtraExclusions {
		if knowledgePathWithin(name, prefix) {
			return true
		}
	}
	return false
}
func directoryOpenRoot(tx *gorm.DB, cfg *directoryRootConfig) (*os.Root, error) {
	root, identity, err := utility.OpenKnowledgeDirectoryRoot(cfg.AbsolutePath)
	if err != nil {
		return nil, directoryError(503, "unavailable", "root_unready")
	}
	fail := func(err error) (*os.Root, error) { root.Close(); return nil, err }
	pathHash := directorySHA([]byte(filepath.Clean(cfg.AbsolutePath)))
	var pin entity.KnowledgeDirectoryRootPin
	err = tx.Where("id = ?", cfg.RootID).Take(&pin).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		pin = entity.KnowledgeDirectoryRootPin{ID: cfg.RootID, DomainID: cfg.DomainID, UserID: cfg.UserID, PathHash: pathHash, Identity: identity, ConfigHash: directoryConfigHash(cfg), Version: cfg.RootVersion, CreatedAt: time.Now().UTC()}
		if err = tx.Create(&pin).Error; err != nil {
			return fail(err)
		}
	} else if err != nil {
		return fail(err)
	}
	if pin.DomainID != cfg.DomainID || pin.UserID != cfg.UserID || pin.PathHash != pathHash || pin.Identity != identity {
		return fail(directoryError(503, "unavailable", "root_unready"))
	}
	if err = directoryValidatePin(tx, cfg); err != nil {
		return fail(err)
	}
	return root, nil
}

func directoryConfigHash(cfg *directoryRootConfig) string {
	exclusions := slices.Clone(cfg.ExtraExclusions)
	if exclusions == nil {
		exclusions = []string{}
	}
	slices.Sort(exclusions)
	return directorySHA([]byte(directoryJSON(exclusions)))
}
func directoryValidatePin(tx *gorm.DB, cfg *directoryRootConfig) error {
	var pin entity.KnowledgeDirectoryRootPin
	if err := tx.Where("id = ?", cfg.RootID).Take(&pin).Error; err != nil {
		return err
	}
	hash := directoryConfigHash(cfg)
	if pin.DomainID != cfg.DomainID || pin.UserID != cfg.UserID || pin.PathHash != directorySHA([]byte(filepath.Clean(cfg.AbsolutePath))) || cfg.RootVersion < pin.Version || (cfg.RootVersion == pin.Version && pin.ConfigHash != hash) {
		return directoryError(503, "unavailable", "root_unready")
	}
	if cfg.RootVersion > pin.Version {
		return tx.Model(&pin).Updates(map[string]any{"version": cfg.RootVersion, "config_hash": hash}).Error
	}
	return nil
}

type directoryScope struct {
	Native entity.KnowledgeSourceRegistration
	View   entity.KnowledgeDirectoryRegistration
	Config *directoryRootConfig
}

func loadDirectoryScope(tx *gorm.DB, user, domain, registration string) (*directoryScope, error) {
	if !isKnowledgeID(registration, 32) {
		return nil, directoryMissing()
	}
	var reg entity.KnowledgeSourceRegistration
	if err := tx.Where("id = ? AND domain_id = ? AND source_kind = ? AND input_mode = ?", registration, domain, "controlled_directory", "server_read").Take(&reg).Error; err != nil {
		return nil, err
	}
	var binding entity.KnowledgeDirectoryRegistrationBinding
	if err := tx.Where("registration_id = ?", reg.ID).Take(&binding).Error; err != nil {
		return nil, err
	}
	cfg, err := directoryConfig(user, domain, binding.RootID)
	if err != nil {
		return nil, err
	}
	if err = directoryValidatePin(tx, cfg); err != nil {
		return nil, err
	}
	view := entity.KnowledgeDirectoryRegistration{RegistrationID: reg.ID, DomainID: reg.DomainID, RegistrationKey: reg.RegistrationKey, Name: reg.Name, SourceKind: reg.SourceKind, InputMode: reg.InputMode, RootID: binding.RootID, ScopeDirectories: reg.IncludePaths, DefaultClassification: entity.KnowledgeDirectoryClassification{GroupID: reg.GroupID, ComponentSide: reg.ComponentSide, Language: reg.Language}, CreatedAt: directoryTime(reg.CreatedAt)}
	return &directoryScope{Native: reg, View: view, Config: cfg}, nil
}
func (s *directoryScope) contains(name string) bool {
	if directoryExcluded(s.Config, name) {
		return false
	}
	for _, prefix := range s.View.ScopeDirectories {
		if knowledgePathWithin(name, prefix) {
			return true
		}
	}
	return false
}

// directoryTransaction serializes result lookups and publishers on the same database row.
func (d *DatasetService) directoryTransaction(ctx context.Context, user, domain string, timeout time.Duration, fn func(*gorm.DB, *entity.Knowledgebase) error) error {
	if _, err := d.knowledgeBase(ctx, user, domain, true); err != nil {
		return directoryNormalizeError(err)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	err := dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		kb, err := lockKnowledgeImportDomain(ctx, tx, user, domain)
		if err != nil {
			return err
		}
		return fn(tx, kb)
	})
	return directoryNormalizeError(err)
}
func (d *DatasetService) ListKnowledgeDirectoryRoots(ctx context.Context, user, domain string) (*entity.KnowledgeDirectoryRoots, error) {
	result := &entity.KnowledgeDirectoryRoots{DomainID: domain, Items: []entity.KnowledgeDirectoryRoot{}}
	err := d.directoryTransaction(ctx, user, domain, 10*time.Second, func(tx *gorm.DB, _ *entity.Knowledgebase) error {
		roots, err := directoryRoots()
		if err != nil {
			return err
		}
		for _, cfg := range roots {
			if cfg.UserID != user || cfg.DomainID != domain {
				continue
			}
			item := entity.KnowledgeDirectoryRoot{RootID: cfg.RootID, RootVersion: cfg.RootVersion, DisplayName: cfg.DisplayName, RuntimePlatform: runtime.GOOS}
			if !utility.KnowledgeDirectoryPlatformVerified() {
				item.Reason = directoryString("platform_unverified")
			} else if root, err := directoryOpenRoot(tx, &cfg); err == nil {
				item.Ready = true
				root.Close()
			} else {
				item.Reason = directoryString("unavailable")
			}
			result.Items = append(result.Items, item)
		}
		slices.SortFunc(result.Items, func(a, b entity.KnowledgeDirectoryRoot) int { return strings.Compare(a.RootID, b.RootID) })
		return nil
	})
	return result, err
}
func (d *DatasetService) CreateKnowledgeDirectoryRegistration(ctx context.Context, user, domain string, req entity.KnowledgeDirectoryRegistrationRequest) (*entity.KnowledgeDirectoryRegistration, bool, error) {
	if !knowledgeImportKey.MatchString(req.RegistrationKey) || len(req.RegistrationKey) > 64 || !validKnowledgeText(req.Name, 128) || req.SourceKind != "controlled_directory" || req.InputMode != "server_read" || len(req.ScopeDirectories) < 1 || len(req.ScopeDirectories) > 16 {
		return nil, false, directoryInvalid()
	}
	req.ScopeDirectories = slices.Clone(req.ScopeDirectories)
	slices.Sort(req.ScopeDirectories)
	for i, p := range req.ScopeDirectories {
		if !utility.IsKnowledgeDirectoryPath(p, true) {
			return nil, false, directoryInvalid()
		}
		for _, prior := range req.ScopeDirectories[:i] {
			if knowledgePathWithin(p, prior) {
				return nil, false, directoryInvalid()
			}
		}
	}
	var result *entity.KnowledgeDirectoryRegistration
	created := false
	err := d.directoryTransaction(ctx, user, domain, 10*time.Second, func(tx *gorm.DB, _ *entity.Knowledgebase) error {
		cfg, err := directoryConfig(user, domain, req.RootID)
		if err != nil {
			return err
		}
		root, err := directoryOpenRoot(tx, cfg)
		if err != nil {
			return err
		}
		root.Close()
		for _, p := range req.ScopeDirectories {
			if directoryExcluded(cfg, p) {
				return directoryInvalid()
			}
		}
		c := req.DefaultClassification
		if err := knowledgeImportClassification(tx, domain, c.GroupID, c.ComponentSide, c.Language); err != nil {
			return err
		}
		hash := directorySHA([]byte(directoryJSON(req)))
		var reg entity.KnowledgeSourceRegistration
		err = tx.Where("domain_id = ? AND registration_key = ?", domain, req.RegistrationKey).Take(&reg).Error
		if err == nil {
			if reg.ConfigHash != hash || reg.SourceKind != "controlled_directory" {
				return directoryConflict("idempotency_conflict")
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		} else {
			reg = entity.KnowledgeSourceRegistration{ID: common.GenerateUUID(), DomainID: domain, RegistrationKey: req.RegistrationKey, Name: req.Name, SourceKind: req.SourceKind, InputMode: req.InputMode, IncludePaths: req.ScopeDirectories, GroupID: c.GroupID, ComponentSide: c.ComponentSide, Language: c.Language, ConfigHash: hash, CreatedAt: time.Now().UTC()}
			if err = tx.Create(&reg).Error; err != nil {
				return err
			}
			if err = tx.Create(&entity.KnowledgeDirectoryRegistrationBinding{RegistrationID: reg.ID, RootID: req.RootID}).Error; err != nil {
				return err
			}
			created = true
		}
		scope, err := loadDirectoryScope(tx, user, domain, reg.ID)
		if err != nil {
			return err
		}
		result = &scope.View
		return nil
	})
	return result, created, err
}
func (d *DatasetService) GetKnowledgeDirectoryRegistration(ctx context.Context, user, domain, registration string) (*entity.KnowledgeDirectoryRegistration, error) {
	var result *entity.KnowledgeDirectoryRegistration
	err := d.directoryTransaction(ctx, user, domain, 10*time.Second, func(tx *gorm.DB, _ *entity.Knowledgebase) error {
		scope, err := loadDirectoryScope(tx, user, domain, registration)
		if err == nil {
			result = &scope.View
		}
		return err
	})
	return result, err
}
func directorySaveState(tx *gorm.DB, user, domain, registration, kind, id string, expires time.Time, payload any, private any) error {
	limit := int64(4)
	if kind == "listing" {
		limit = 8
	}
	var count int64
	if err := tx.Model(&entity.KnowledgeDirectoryState{}).Where("domain_id = ? AND user_id = ? AND kind = ? AND expires_at > ?", domain, user, kind, time.Now().UTC()).Count(&count).Error; err != nil {
		return err
	}
	if count >= limit {
		return directoryError(429, "capacity", "")
	}
	row := entity.KnowledgeDirectoryState{ID: id, DomainID: domain, UserID: user, RegistrationID: registration, Kind: kind, Payload: directoryJSON(payload), PrivateData: directoryJSON(private), ExpiresAt: expires, CreatedAt: time.Now().UTC()}
	return tx.Create(&row).Error
}
func directoryLoadState(tx *gorm.DB, scope *directoryScope, user, kind, id string, allowExpired bool) (*entity.KnowledgeDirectoryState, error) {
	if !isKnowledgeID(id, 32) {
		return nil, directoryMissing()
	}
	var row entity.KnowledgeDirectoryState
	if err := tx.Where("id = ? AND registration_id = ? AND domain_id = ? AND user_id = ? AND kind = ?", id, scope.Native.ID, scope.Native.DomainID, user, kind).Take(&row).Error; err != nil {
		return nil, err
	}
	if !allowExpired && !time.Now().Before(row.ExpiresAt) {
		return nil, directoryError(410, "expired", "")
	}
	return &row, nil
}
func directoryCheckVersions(scope *directoryScope, kb *entity.Knowledgebase, rootVersion, policyVersion int64) error {
	if rootVersion != scope.Config.RootVersion {
		return directoryConflict("root_changed")
	}
	if policyVersion != 0 && policyVersion != kb.KnowledgePolicyVersion {
		return directoryConflict("policy_changed")
	}
	return nil
}

// StartKnowledgeDirectoryCleanup clears expired temporary bodies independently of browser traffic.
func StartKnowledgeDirectoryCleanup(ctx context.Context) {
	if os.Getenv("LFEN_KNOWLEDGE_ROOTS_FILE") == "" {
		return
	}
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			if err := CleanupKnowledgeDirectoryStates(ctx, time.Now().UTC()); err != nil && ctx.Err() == nil {
				common.Warn("Knowledge directory temporary cleanup failed")
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}
func CleanupKnowledgeDirectoryStates(ctx context.Context, now time.Time) error {
	if dao.DB == nil {
		return nil
	}
	db := dao.DB.WithContext(ctx)
	previewIDs := db.Model(&entity.KnowledgeDirectoryState{}).Select("id").Where("kind = ? AND expires_at <= ?", "preview", now)
	if err := db.Model(&entity.KnowledgeImportPreview{}).Where("id IN (?)", previewIDs).Update("content", "").Error; err != nil {
		return err
	}
	if err := db.Model(&entity.KnowledgeDirectoryState{}).Where("kind IN ? AND expires_at <= ?", []string{"listing", "observation"}, now).Updates(map[string]any{"payload": "", "private_data": ""}).Error; err != nil {
		return err
	}
	old := now.Add(-24 * time.Hour)
	ids := db.Model(&entity.KnowledgeDirectoryState{}).Select("id").Where("kind = ? AND expires_at <= ?", "preview", old)
	if err := db.Where("id IN (?) AND consumed_task_id IS NULL", ids).Delete(&entity.KnowledgeImportPreview{}).Error; err != nil {
		return err
	}
	consumed := db.Model(&entity.KnowledgeImportPreview{}).Select("id").Where("consumed_task_id IS NOT NULL")
	return db.Where("expires_at <= ? AND (kind <> ? OR id NOT IN (?))", old, "preview", consumed).Delete(&entity.KnowledgeDirectoryState{}).Error
}
func directoryTokens(tokens ...any) string {
	var encoded strings.Builder
	for _, token := range tokens {
		value := ""
		switch x := token.(type) {
		case nil:
		case *string:
			if x != nil {
				value = *x
			}
		case *int64:
			if x != nil {
				value = fmt.Sprint(*x)
			}
		default:
			value = fmt.Sprint(x)
		}
		fmt.Fprintf(&encoded, "%d:%s", len(value), value)
	}
	return encoded.String()
}
