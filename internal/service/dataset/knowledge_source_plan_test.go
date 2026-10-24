package dataset

import (
	"os"
	"path/filepath"
	"testing"
)

func TestKnowledgeSourcePreparationIncrementalPlan(t *testing.T) {
	root := t.TempDir()
	for _, file := range []string{"web/same.ts", "web/update.ts", "web/missing.ts", "web/rename.ts"} {
		writePreparationSource(t, root, file, []byte("same original bytes"))
	}
	options := preparationOptions()
	before := requirePreparationManifest(t, root, options)
	writePreparationSource(t, root, "web/update.ts", []byte("new bytes"))
	writePreparationSource(t, root, "web/new.ts", []byte("same original bytes"))
	for _, name := range []string{"web/missing.ts", "web/rename.ts"} {
		if err := os.Remove(filepath.Join(root, filepath.FromSlash(name))); err != nil {
			t.Fatal(err)
		}
	}
	writePreparationSource(t, root, "web/renamed.ts", []byte("same original bytes"))
	after := requirePreparationManifest(t, root, options)
	plan, err := planKnowledgeSources(before, after)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"web/same.ts": "unchanged", "web/update.ts": "update", "web/missing.ts": "missing_proposal", "web/rename.ts": "missing_proposal", "web/renamed.ts": "add", "web/new.ts": "add"}
	if len(plan.Changes) != len(want) || plan.ScopeChanged {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	for _, change := range plan.Changes {
		if change.Action != want[change.Path] {
			t.Fatalf("wrong action: %+v", change)
		}
	}
	repeated, err := planKnowledgeSources(before, after)
	if err != nil || repeated.SHA256 != plan.SHA256 {
		t.Fatal("same dry-run changed")
	}
	// Planning does not modify either the files or the observations.
	if err := verifyKnowledgeCollection(t.Context(), root, options, after); err != nil {
		t.Fatal(err)
	}
	other := *after
	other.RegistrationKey = "other-registration"
	if _, err := planKnowledgeSources(before, &other); err == nil {
		t.Fatal("cross-registration hash dedup accepted")
	}
}

func TestKnowledgeSourcePreparationIncompleteAndScopeChanges(t *testing.T) {
	root := t.TempDir()
	writePreparationSource(t, root, "web/a.ts", []byte("a"))
	writePreparationSource(t, root, "internal/a.go", []byte("b"))
	options := preparationOptions()
	before := requirePreparationManifest(t, root, options)
	options.IncludePaths = []string{"web"}
	after := requirePreparationManifest(t, root, options)
	plan, err := planKnowledgeSources(before, after)
	if err != nil || !plan.ScopeChanged {
		t.Fatalf("scope change lost: %+v %v", plan, err)
	}
	if plan.Changes[0].Path != "internal/a.go" || plan.Changes[0].Action != "unobserved" {
		t.Fatalf("scope shrink treated as deletion: %+v", plan)
	}
	options = preparationOptions()
	writePreparationSource(t, root, "web/a.ts", []byte{0xff})
	after = requirePreparationManifest(t, root, options)
	plan, err = planKnowledgeSources(before, after)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range plan.Changes {
		if change.Path == "web/a.ts" && change.Action != "blocked" {
			t.Fatal("invalid update became a missing/deleted source")
		}
	}
	if err := verifyKnowledgeCollection(t.Context(), root, options, after); err == nil {
		t.Fatal("incomplete observation admitted")
	}
	// Metadata/classification changes are observable even when bytes are equal.
	writePreparationSource(t, root, "web/a.ts", []byte("a"))
	options.Rules = append(options.Rules, knowledgeSourceRule{Prefix: "web/a.ts", ComponentSide: "shared", Language: "typescript"})
	after = requirePreparationManifest(t, root, options)
	plan, err = planKnowledgeSources(before, after)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range plan.Changes {
		if change.Path == "web/a.ts" && change.Action != "update" {
			t.Fatal("classification edit ignored")
		}
	}
	after.Items[0].SHA256 = "tampered"
	if _, err := planKnowledgeSources(before, after); err == nil {
		t.Fatal("modified manifest admitted")
	}
}
