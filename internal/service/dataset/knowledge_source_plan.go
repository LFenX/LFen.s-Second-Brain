package dataset

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

type knowledgeSourceChange struct {
	Path   string
	Action string
	Before *knowledgeSourceItem
	After  *knowledgeSourceItem
}

type knowledgeSourcePlan struct {
	BeforeSHA256 string
	AfterSHA256  string
	ScopeChanged bool
	Changes      []knowledgeSourceChange
	SHA256       string
}

// planKnowledgeSources produces a dry-run, never source IDs or write operations.
// Missing paths are proposals even when both observations have the same scope.
func planKnowledgeSources(before, after *knowledgeSourceManifest) (*knowledgeSourcePlan, error) {
	if after == nil || after.RegistrationKey == "" || after.SHA256 == "" {
		return nil, fmt.Errorf("an observed manifest is required")
	}
	if before != nil && before.RegistrationKey != after.RegistrationKey {
		return nil, fmt.Errorf("cannot compare different registration scopes")
	}
	if err := validateKnowledgeManifest(after); err != nil {
		return nil, err
	}
	result := &knowledgeSourcePlan{AfterSHA256: after.SHA256, Changes: []knowledgeSourceChange{}}
	previous := make(map[string]knowledgeSourceItem)
	if before != nil {
		if err := validateKnowledgeManifest(before); err != nil {
			return nil, err
		}
		result.BeforeSHA256 = before.SHA256
		result.ScopeChanged = before.ScopeSHA256 != after.ScopeSHA256
		for _, item := range before.Items {
			previous[item.Path] = item
		}
	}
	for _, item := range after.Items {
		old, exists := previous[item.Path]
		change := knowledgeSourceChange{Path: item.Path, After: &item, Action: "add"}
		if exists {
			change.Before = &old
			change.Action = "update"
			if old.State == "observed" && old == item {
				change.Action = "unchanged"
			}
		}
		if item.State != "observed" {
			change.Action = "blocked"
		}
		result.Changes = append(result.Changes, change)
		delete(previous, item.Path)
	}
	for _, item := range previous {
		action := "missing_proposal"
		if result.ScopeChanged || !after.Complete {
			action = "unobserved"
		}
		result.Changes = append(result.Changes, knowledgeSourceChange{Path: item.Path, Before: &item, Action: action})
	}
	slices.SortFunc(result.Changes, func(a, b knowledgeSourceChange) int { return strings.Compare(a.Path, b.Path) })
	result.SHA256 = knowledgePreparationHash(result)
	return result, nil
}

func validateKnowledgeManifest(manifest *knowledgeSourceManifest) error {
	copy := *manifest
	copy.SHA256 = ""
	if manifest.SHA256 != knowledgePreparationHash(&copy) {
		return fmt.Errorf("manifest digest mismatch")
	}
	seen := make(map[string]bool)
	for _, item := range manifest.Items {
		if !isKnowledgeImportPath(item.Path) || seen[item.Path] {
			return fmt.Errorf("invalid or duplicate manifest path")
		}
		seen[item.Path] = true
	}
	return nil
}

// verifyKnowledgeCollection re-observes bytes before any future admission step.
// It does not turn two filesystem observations into an atomic project snapshot.
func verifyKnowledgeCollection(ctx context.Context, root string, options knowledgeCollectionOptions, expected *knowledgeSourceManifest) error {
	if expected == nil || !expected.Complete {
		return fmt.Errorf("incomplete manifest cannot be admitted")
	}
	if err := validateKnowledgeManifest(expected); err != nil {
		return err
	}
	current, err := collectKnowledgeSources(ctx, root, options)
	if err != nil {
		return err
	}
	if !current.Complete || current.SHA256 != expected.SHA256 {
		return fmt.Errorf("sources changed since collection; collect and confirm again")
	}
	return nil
}
