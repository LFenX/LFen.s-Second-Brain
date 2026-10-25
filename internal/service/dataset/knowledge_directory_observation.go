package dataset

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"io"
	"os"
	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/utility"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type directoryListingPrivate struct {
	Cursors []string `json:"cursors"`
}

func directoryListingPage(view entity.KnowledgeDirectoryListing, private directoryListingPrivate, cursor string) (*entity.KnowledgeDirectoryListing, error) {
	page := 0
	if cursor != "" {
		page = slices.Index(private.Cursors, cursor)
		if page < 1 {
			return nil, directoryInvalid()
		}
	}
	start := page * 50
	if start > len(view.Items) {
		return nil, directoryInvalid()
	}
	end := min(start+50, len(view.Items))
	view.Items = slices.Clone(view.Items[start:end])
	if view.Items == nil {
		view.Items = []entity.KnowledgeDirectoryEntry{}
	}
	view.NextCursor = nil
	if end < int(view.TotalEntries) {
		view.NextCursor = &private.Cursors[page+1]
	}
	return &view, nil
}
func (d *DatasetService) CreateKnowledgeDirectoryListing(ctx context.Context, user, domain, registration string, req entity.KnowledgeDirectoryListingRequest) (*entity.KnowledgeDirectoryListing, error) {
	if !utility.IsKnowledgeDirectoryPath(req.RelativeDirectory, true) || len(req.NamePrefix) > 128 || !utf8.ValidString(req.NamePrefix) || strings.ContainsAny(req.NamePrefix, "/\\:*?[]{}") || strings.ContainsFunc(req.NamePrefix, unicode.IsControl) {
		return nil, directoryInvalid()
	}
	var result *entity.KnowledgeDirectoryListing
	err := d.directoryTransaction(ctx, user, domain, 10*time.Second, func(tx *gorm.DB, kb *entity.Knowledgebase) error {
		scope, err := loadDirectoryScope(tx, user, domain, registration)
		if err != nil {
			return err
		}
		if !scope.contains(req.RelativeDirectory) {
			return directoryInvalid()
		}
		root, err := directoryOpenRoot(tx, scope.Config)
		if err != nil {
			return err
		}
		defer root.Close()
		readCtx, cancel := context.WithTimeout(tx.Statement.Context, 8*time.Second)
		defer cancel()
		f, before, _, err := utility.OpenKnowledgeDirectoryEntry(readCtx, root, req.RelativeDirectory, true)
		if err != nil {
			return directoryInvalid()
		}
		defer f.Close()
		now := time.Now().UTC()
		expires := now.Add(10 * time.Minute)
		view := entity.KnowledgeDirectoryListing{ListingID: common.GenerateUUID(), DomainID: domain, RegistrationID: registration, RootID: scope.Config.RootID, RootVersion: scope.Config.RootVersion, RelativeDirectory: req.RelativeDirectory, NamePrefix: req.NamePrefix, CapturedAt: directoryTime(now), ExpiresAt: directoryTime(expires), Complete: true, Items: []entity.KnowledgeDirectoryEntry{}}
		stop := false
		for !stop {
			if readCtx.Err() != nil {
				view.Complete = false
				view.Reason = directoryString("timeout")
				break
			}
			entries, e := f.ReadDir(128)
			for _, entry := range entries {
				if !strings.HasPrefix(entry.Name(), req.NamePrefix) {
					continue
				}
				relative := entry.Name()
				if req.RelativeDirectory != "." {
					relative = req.RelativeDirectory + "/" + relative
				}
				if directoryExcluded(scope.Config, relative) {
					continue
				}
				if len(view.Items) >= 512 {
					view.Complete = false
					view.Reason = directoryString("limit_reached")
					stop = true
					break
				}
				item := entity.KnowledgeDirectoryEntry{RelativePath: relative, EntryKind: "blocked", Reason: directoryString("unreadable")}
				info, reason := utility.InspectKnowledgeDirectoryEntry(root, relative)
				if reason != "" {
					item.Reason = directoryString(reason)
				} else if info.IsDir() {
					item.EntryKind = "directory"
					item.Reason = nil
				} else {
					item.EntryKind = "file"
					item.Selectable = true
					item.Reason = nil
					item.ByteLength = directoryInt(info.Size())
				}
				view.Items = append(view.Items, item)
			}
			if e == io.EOF {
				break
			}
			if e != nil {
				view.Complete = false
				view.Reason = directoryString("read_error")
				break
			}
		}
		after, e := f.Stat()
		current, e2 := root.Lstat(req.RelativeDirectory)
		if e != nil || e2 != nil || !os.SameFile(before, after) || !os.SameFile(after, current) || !before.ModTime().Equal(after.ModTime()) {
			view.Complete = false
			view.Reason = directoryString("changed_during_read")
		}
		slices.SortFunc(view.Items, func(a, b entity.KnowledgeDirectoryEntry) int { return strings.Compare(a.RelativePath, b.RelativePath) })
		view.TotalEntries = int64(len(view.Items))
		private := directoryListingPrivate{Cursors: []string{""}}
		for n := 50; n < len(view.Items); n += 50 {
			private.Cursors = append(private.Cursors, common.GenerateUUID())
		}
		if err := directorySaveState(tx, user, domain, registration, "listing", view.ListingID, expires, view, private); err != nil {
			return err
		}
		result, err = directoryListingPage(view, private, "")
		return err
	})
	return result, err
}
func (d *DatasetService) GetKnowledgeDirectoryListing(ctx context.Context, user, domain, registration, id, cursor string) (*entity.KnowledgeDirectoryListing, error) {
	var result *entity.KnowledgeDirectoryListing
	err := d.directoryTransaction(ctx, user, domain, 10*time.Second, func(tx *gorm.DB, kb *entity.Knowledgebase) error {
		scope, err := loadDirectoryScope(tx, user, domain, registration)
		if err != nil {
			return err
		}
		row, err := directoryLoadState(tx, scope, user, "listing", id, false)
		if err != nil {
			return err
		}
		var view entity.KnowledgeDirectoryListing
		var private directoryListingPrivate
		if json.Unmarshal([]byte(row.Payload), &view) != nil || json.Unmarshal([]byte(row.PrivateData), &private) != nil {
			return directoryInvalid()
		}
		if err = directoryCheckVersions(scope, kb, view.RootVersion, 0); err != nil {
			return err
		}
		result, err = directoryListingPage(view, private, cursor)
		return err
	})
	return result, err
}

type directoryObservationPrivate struct {
	Identities map[string]string `json:"identities"`
}

func directoryObservationDigest(view *entity.KnowledgeDirectoryObservation) string {
	c := view.DefaultClassification
	tokens := []any{"C2-DIR-OBS-1", view.ObservationID, view.DomainID, view.RegistrationID, view.RootID, view.RootVersion, view.PolicyVersion, c.GroupID, c.ComponentSide, c.Language, view.CapturedAt, view.ExpiresAt, view.Complete, len(view.SelectedPaths)}
	for _, p := range view.SelectedPaths {
		tokens = append(tokens, p)
	}
	tokens = append(tokens, len(view.Items))
	for _, i := range view.Items {
		tokens = append(tokens, i.RelativePath, i.State, i.Eligible, i.Reason, i.ByteLength, i.SHA256, i.SourceID, i.ExpectedRevisionID, i.ChangeKind)
	}
	return directorySHA([]byte(directoryTokens(tokens...)))
}
func directoryCurrent(tx *gorm.DB, domain, registration, relative string) (*entity.KnowledgeSourceBinding, *entity.KnowledgeSourceRevision, error) {
	var binding entity.KnowledgeSourceBinding
	err := tx.Where("registration_id = ? AND relative_path = ?", registration, relative).Take(&binding).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	current, err := knowledgeImportCurrent(tx, domain, binding.DocumentID)
	return &binding, current, err
}
func directoryTextReason(name string, content []byte) string {
	if !utility.IsKnowledgeTextSource(name, nil) {
		return "unsupported_format"
	}
	if !utf8.Valid(content) {
		return "invalid_utf8"
	}
	if bytes.IndexByte(content, 0) >= 0 {
		return "nul_byte"
	}
	if bytes.Contains(content, []byte("-----BEGIN PRIVATE KEY-----")) || bytes.Contains(content, []byte("-----BEGIN RSA PRIVATE KEY-----")) {
		return "secret_marker"
	}
	return ""
}
func directoryReadReason(err error) string {
	if errors.Is(err, os.ErrNotExist) {
		return "missing"
	}
	switch err.Error() {
	case "link", "hardlink", "not_regular", "too_large", "changed_during_read", "identity_unverified":
		return err.Error()
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "timeout"
	}
	return "unreadable"
}
func directoryAction(sha, relative string, c entity.KnowledgeDirectoryClassification, current *entity.KnowledgeSourceRevision) string {
	if current == nil {
		return "add"
	}
	req := entity.SnapshotKnowledgeSourceRequest{GroupID: c.GroupID, RelativePath: relative, ComponentSide: c.ComponentSide, Language: c.Language}
	if current.InputHash == knowledgeSourceInputHash(sha, req) {
		return "unchanged"
	}
	return "update"
}
func (d *DatasetService) CreateKnowledgeDirectoryObservation(ctx context.Context, user, domain, registration string, req entity.KnowledgeDirectoryObservationRequest) (*entity.KnowledgeDirectoryObservation, error) {
	if len(req.SelectedPaths) < 1 || len(req.SelectedPaths) > 32 {
		return nil, directoryInvalid()
	}
	paths := slices.Clone(req.SelectedPaths)
	slices.Sort(paths)
	for i, p := range paths {
		if !utility.IsKnowledgeDirectoryPath(p, false) || (i > 0 && p == paths[i-1]) {
			return nil, directoryInvalid()
		}
	}
	var result *entity.KnowledgeDirectoryObservation
	err := d.directoryTransaction(ctx, user, domain, 30*time.Second, func(tx *gorm.DB, kb *entity.Knowledgebase) error {
		scope, err := loadDirectoryScope(tx, user, domain, registration)
		if err != nil {
			return err
		}
		for _, p := range paths {
			if !scope.contains(p) {
				return directoryInvalid()
			}
		}
		root, err := directoryOpenRoot(tx, scope.Config)
		if err != nil {
			return err
		}
		defer root.Close()
		readCtx, cancel := context.WithTimeout(tx.Statement.Context, 28*time.Second)
		defer cancel()
		now := time.Now().UTC()
		expires := now.Add(10 * time.Minute)
		view := entity.KnowledgeDirectoryObservation{ObservationID: common.GenerateUUID(), DomainID: domain, RegistrationID: registration, RootID: scope.Config.RootID, RootVersion: scope.Config.RootVersion, PolicyVersion: kb.KnowledgePolicyVersion, DefaultClassification: scope.View.DefaultClassification, SelectedPaths: paths, Complete: true, CapturedAt: directoryTime(now), ExpiresAt: directoryTime(expires), Items: []entity.KnowledgeDirectoryObservationItem{}}
		private := directoryObservationPrivate{Identities: map[string]string{}}
		for _, p := range paths {
			item := entity.KnowledgeDirectoryObservationItem{RelativePath: p, State: "unobserved", ChangeKind: "unobserved", Reason: directoryString("unreadable")}
			binding, current, e := directoryCurrent(tx, domain, registration, p)
			if e != nil {
				return e
			}
			f, info, _, e := utility.OpenKnowledgeDirectoryEntry(readCtx, root, p, false)
			if e != nil {
				reason := directoryReadReason(e)
				item.Reason = &reason
				if reason == "missing" {
					item.State = "missing"
					if binding != nil {
						item.ChangeKind = "missing_proposal"
						item.SourceID = &binding.DocumentID
						item.ExpectedRevisionID = &current.ID
					}
				} else if slices.Contains([]string{"link", "hardlink", "not_regular", "identity_unverified"}, reason) {
					item.State = "ineligible"
					item.ChangeKind = "blocked"
				}
			} else {
				f.Close()
				item.ByteLength = directoryInt(info.Size())
				reason := ""
				if info.Size() > entity.KnowledgeMaxSourceBytes {
					reason = "too_large"
				} else if !utility.IsKnowledgeTextSource(p, nil) {
					reason = "unsupported_format"
				}
				if reason != "" {
					item.State = "ineligible"
					item.ChangeKind = "blocked"
					item.Reason = &reason
				} else {
					content, identity, readErr := utility.ReadKnowledgeDirectoryFile(readCtx, root, p, entity.KnowledgeMaxSourceBytes)
					if readErr != nil {
						reason = directoryReadReason(readErr)
						item.ByteLength = nil
						item.Reason = &reason
						if reason == "too_large" {
							item.State = "ineligible"
							item.ChangeKind = "blocked"
						}
					} else if reason = directoryTextReason(p, content); reason != "" {
						item.State = "ineligible"
						item.ChangeKind = "blocked"
						item.Reason = &reason
					} else {
						item.State = "verified"
						item.Eligible = true
						item.Reason = nil
						item.SHA256 = directoryString(directorySHA(content))
						item.ByteLength = directoryInt(int64(len(content)))
						item.ChangeKind = directoryAction(*item.SHA256, p, scope.View.DefaultClassification, current)
						if binding != nil {
							item.SourceID = &binding.DocumentID
							item.ExpectedRevisionID = &current.ID
						}
						private.Identities[p] = identity
					}
				}
			}
			if item.State == "missing" || item.State == "unobserved" {
				view.Complete = false
			}
			view.Items = append(view.Items, item)
		}
		view.ObservationSHA256 = directoryObservationDigest(&view)
		if err := directorySaveState(tx, user, domain, registration, "observation", view.ObservationID, expires, view, private); err != nil {
			return err
		}
		result = &view
		return nil
	})
	return result, err
}
func loadDirectoryObservation(tx *gorm.DB, scope *directoryScope, user, id string, kb *entity.Knowledgebase) (*entity.KnowledgeDirectoryObservation, *directoryObservationPrivate, error) {
	row, err := directoryLoadState(tx, scope, user, "observation", id, false)
	if err != nil {
		return nil, nil, err
	}
	var view entity.KnowledgeDirectoryObservation
	var private directoryObservationPrivate
	if json.Unmarshal([]byte(row.Payload), &view) != nil || json.Unmarshal([]byte(row.PrivateData), &private) != nil {
		return nil, nil, directoryInvalid()
	}
	if err = directoryCheckVersions(scope, kb, view.RootVersion, view.PolicyVersion); err != nil {
		return nil, nil, err
	}
	if directoryObservationDigest(&view) != view.ObservationSHA256 {
		return nil, nil, directoryConflict("observation_mismatch")
	}
	return &view, &private, nil
}
func (d *DatasetService) GetKnowledgeDirectoryObservation(ctx context.Context, user, domain, registration, id string) (*entity.KnowledgeDirectoryObservation, error) {
	var result *entity.KnowledgeDirectoryObservation
	err := d.directoryTransaction(ctx, user, domain, 10*time.Second, func(tx *gorm.DB, kb *entity.Knowledgebase) error {
		scope, err := loadDirectoryScope(tx, user, domain, registration)
		if err != nil {
			return err
		}
		result, _, err = loadDirectoryObservation(tx, scope, user, id, kb)
		return err
	})
	return result, err
}
