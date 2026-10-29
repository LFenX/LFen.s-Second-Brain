//go:build linux

package parser

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// The private child flag is intercepted only in this test binary. No server,
// native parser, network service, SQL connection or model is initialized here.
func init() {
	if IsDocumentStructureChild(os.Args[1:]) {
		os.Exit(DocumentStructureChildMain(context.Background(), func(ctx context.Context, control DocumentChildControl, input []byte) (json.RawMessage, error) {
			if control.ProfileID == KnowledgeParsedLocalProfile {
				return ParseKnowledgeParsedChild(ctx, control, input)
			}
			switch string(input) {
			case "test:block":
				<-ctx.Done()
				return nil, ctx.Err()
			case "test:uncancellable":
				for {
					time.Sleep(time.Second)
				}
			case "test:panic":
				panic("raw-native-private-secret-must-not-escape")
			case "test:oversized":
				return json.RawMessage(`{"large":"` + strings.Repeat("a", documentChildOutputLimit) + `"}`), nil
			case "test:stdout":
				fmt.Fprintln(os.Stdout, "native-private-stdout-must-not-be-a-frame")
			case "test:stderr":
				fmt.Fprint(os.Stderr, strings.Repeat("native-private-stderr", documentChildStderrLimit))
			}
			return json.Marshal(map[string]any{"text": string(input), "token_present": documentChildHash(os.Getenv("LFEN_DOCUMENT_CHILD_TOKEN"), 64), "has_model_key": os.Getenv("DASHSCOPE_API_KEY") != "" || os.Getenv("DEEPSEEK_API_KEY") != "", "has_proxy": os.Getenv("HTTP_PROXY") != "" || os.Getenv("HTTPS_PROXY") != "" || os.Getenv("ALL_PROXY") != "", "profile": control.ProfileID})
		}))
	}
}

func documentChildOwnedTestInput(t *testing.T, value string) ([]byte, DocumentChildControl) {
	t.Helper()
	if !DocumentStructureChildAvailable() {
		t.Skip("Linux pidfd is unavailable; production capabilities must remain unavailable")
	}
	input := []byte(value)
	hash := sha256.Sum256(input)
	token := make([]byte, 16)
	if _, err := rand.Read(token); err != nil {
		t.Fatal(err)
	}
	control := documentChildTestControl()
	control.TaskID, control.InputSHA256, control.InputBytes = hex.EncodeToString(token), hex.EncodeToString(hash[:]), int64(len(input))
	return input, control
}

func TestDocumentChildLocalRoundtripAndEnvironment(t *testing.T) {
	t.Setenv("DASHSCOPE_API_KEY", "synthetic-key-must-stay-in-parent")
	t.Setenv("DEEPSEEK_API_KEY", "synthetic-key-must-stay-in-parent")
	t.Setenv("HTTP_PROXY", "http://synthetic.invalid")
	t.Setenv("HTTPS_PROXY", "http://synthetic.invalid")
	t.Setenv("ALL_PROXY", "socks5://synthetic.invalid")
	input, control := documentChildOwnedTestInput(t, "a—b\r\n")
	var observed DocumentChildOwnership
	result, err := RunDocumentStructureChild(t.Context(), input, control, func(owner DocumentChildOwnership) error {
		observed = owner
		stored, err := documentChildLoadOwner(owner.Directory)
		if err != nil || stored != owner || owner.PGID != owner.PID || owner.StartTicks == 0 {
			return errors.New("ownership marker must be durable before native execution")
		}
		file, err := os.Stat(filepath.Join(owner.Directory, "input"))
		if err != nil || file.Mode().Perm() != 0600 || file.Size() != int64(len(input)) {
			return errors.New("input file ownership/budget mismatch")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("roundtrip failed: controlled_error_type=%T controlled_error=%v controlled_failure=%#v", err, err, err)
	}
	var value struct {
		Text         string `json:"text"`
		TokenPresent bool   `json:"token_present"`
		HasModelKey  bool   `json:"has_model_key"`
		HasProxy     bool   `json:"has_proxy"`
	}
	if json.Unmarshal(result, &value) != nil || value.Text != string(input) || !value.TokenPresent || value.HasModelKey || value.HasProxy {
		t.Fatal("local payload or minimal environment changed")
	}
	if _, err := os.Lstat(observed.Directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("successful execution did not remove its owned temporary files")
	}
	if err := RecoverDocumentStructureChild(t.Context(), observed); err != nil {
		t.Fatal("read-only recovery after confirmed exit/cleanup should be idempotent")
	}
}

func TestDocumentChildCancelKillsOwnedGroupAndWaits(t *testing.T) {
	input, control := documentChildOwnedTestInput(t, "test:uncancellable")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var observed DocumentChildOwnership
	result, err := RunDocumentStructureChild(ctx, input, control, func(owner DocumentChildOwnership) error {
		observed = owner
		cancel()
		return nil
	})
	if len(result) != 0 || !errors.Is(err, context.Canceled) || errors.Is(err, ErrDocumentChildUnresolved) {
		t.Fatalf("cancellation did not retain its cause after confirmed stop/wait/cleanup: controlled_error_type=%T controlled_error=%v controlled_failure=%#v", err, err, err)
	}
	if _, err := documentChildProcessInfo(observed.PID); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("cancelled owned process was not reaped by its parent")
	}
	if _, err := os.Lstat(observed.Directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("cancelled execution retained its temporary input")
	}
}

func TestDocumentChildParentDeadlineKillsIgnoringParser(t *testing.T) {
	input, control := documentChildOwnedTestInput(t, "test:uncancellable")
	ctx, cancel := context.WithTimeout(t.Context(), 250*time.Millisecond)
	defer cancel()
	_, err := RunDocumentStructureChild(ctx, input, control, func(DocumentChildOwnership) error { return nil })
	var failure *DocumentChildError
	if !errors.Is(err, context.DeadlineExceeded) || !errors.As(err, &failure) || failure.Code != "native_timeout" || errors.Is(err, ErrDocumentChildUnresolved) {
		t.Fatal("deadline did not enforce owned-process termination independently of parser context")
	}
}

func TestDocumentChildStartRejectionAndInputIntegrity(t *testing.T) {
	t.Run("durable_callback_failure", func(t *testing.T) {
		input, control := documentChildOwnedTestInput(t, "never-admit-native")
		var observed DocumentChildOwnership
		_, err := RunDocumentStructureChild(t.Context(), input, control, func(owner DocumentChildOwnership) error {
			observed = owner
			return errors.New("synthetic SQL ownership-save failure")
		})
		if err == nil || errors.Is(err, ErrDocumentChildUnresolved) || strings.Contains(err.Error(), "SQL") {
			t.Fatal("persistence refusal exposed private diagnostics or lost cleanup proof")
		}
		if _, err := os.Lstat(observed.Directory); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("unadmitted child input was not cleaned")
		}
	})
	t.Run("child_rehashes_readonly_fd", func(t *testing.T) {
		input, control := documentChildOwnedTestInput(t, "fixed-input")
		_, err := RunDocumentStructureChild(t.Context(), input, control, func(owner DocumentChildOwnership) error {
			return os.WriteFile(filepath.Join(owner.Directory, "input"), []byte("changed-input"), 0600)
		})
		var failure *DocumentChildError
		if !errors.As(err, &failure) || failure.Code != "original_integrity" {
			t.Fatalf("child accepted an input changed after the parent SHA calculation: controlled_error_type=%T controlled_error=%v controlled_failure=%#v", err, err, failure)
		}
	})
	t.Run("caller_original_sha", func(t *testing.T) {
		input, control := documentChildOwnedTestInput(t, "fixed-input")
		control.InputSHA256 = strings.Repeat("a", 64)
		called := false
		_, err := RunDocumentStructureChild(t.Context(), input, control, func(DocumentChildOwnership) error { called = true; return nil })
		var failure *DocumentChildError
		if called || !errors.As(err, &failure) || failure.Code != "original_integrity" {
			t.Fatal("parent must check the fixed original SHA before starting a child")
		}
	})
}

func TestDocumentChildNativeDiagnosticsAndPackBudget(t *testing.T) {
	for _, mode := range []string{"test:stdout", "test:stderr", "test:panic", "test:oversized"} {
		t.Run(mode, func(t *testing.T) {
			input, control := documentChildOwnedTestInput(t, mode)
			result, err := RunDocumentStructureChild(t.Context(), input, control, func(DocumentChildOwnership) error { return nil })
			if mode == "test:stdout" || mode == "test:stderr" {
				if err != nil || !json.Valid(result) {
					t.Fatal("native diagnostics corrupted or blocked the private pack")
				}
				return
			}
			var failure *DocumentChildError
			if !errors.As(err, &failure) || bytes.Contains(result, []byte("private")) || strings.Contains(err.Error(), "private") {
				t.Fatal("native diagnostic/panic escaped controlled failure")
			}
			if mode == "test:oversized" && failure.Code != "budget_exceeded" || mode == "test:panic" && failure.Code != "native_process_failed" {
				t.Fatal("controlled failure code changed")
			}
		})
	}
}

func TestDocumentChildWrongOwnershipCannotKillOrClean(t *testing.T) {
	input, control := documentChildOwnedTestInput(t, "test:block")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	started := make(chan DocumentChildOwnership, 1)
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := RunDocumentStructureChild(ctx, input, control, func(owner DocumentChildOwnership) error {
			started <- owner
			<-release
			return nil
		})
		done <- err
	}()
	var owner DocumentChildOwnership
	select {
	case owner = <-started:
	case err := <-done:
		t.Fatalf("owned child returned before durable callback: controlled_error_type=%T controlled_error=%v controlled_failure=%#v", err, err, err)
	case <-time.After(3 * time.Second):
		t.Fatal("owned child did not start")
	}
	for _, kind := range []string{"start_ticks", "token", "directory", "pgid"} {
		changed := owner
		switch kind {
		case "start_ticks":
			changed.StartTicks++
		case "token":
			changed.Token = strings.Repeat("b", 64)
		case "directory":
			changed.Directory = t.TempDir()
		case "pgid":
			changed.PGID = os.Getpid()
		}
		if err := RecoverDocumentStructureChild(t.Context(), changed); !errors.Is(err, ErrDocumentChildUnresolved) {
			t.Fatal("mismatched ownership did not retain its execution fence")
		}
		if err := unix.Kill(owner.PID, 0); err != nil {
			t.Fatal("mismatched ownership signaled the live child")
		}
	}
	close(release)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("cleanup of the correctly owned child failed")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("correctly owned child did not stop")
	}
}

func TestDocumentChildRecoveryStopsOnlySavedOwnedChild(t *testing.T) {
	input, control := documentChildOwnedTestInput(t, "test:block")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	started := make(chan DocumentChildOwnership, 1)
	done := make(chan error, 1)
	go func() {
		_, err := RunDocumentStructureChild(ctx, input, control, func(owner DocumentChildOwnership) error { started <- owner; return nil })
		done <- err
	}()
	var owner DocumentChildOwnership
	select {
	case owner = <-started:
	case err := <-done:
		t.Fatalf("owned recovery child returned before durable callback: controlled_error_type=%T controlled_error=%v controlled_failure=%#v", err, err, err)
	case <-time.After(3 * time.Second):
		t.Fatal("owned child did not start")
	}
	if err := RecoverDocumentStructureChild(t.Context(), owner); err != nil {
		t.Fatal("saved owned child could not be stopped and cleaned")
	}
	select {
	case err := <-done:
		if err == nil || errors.Is(err, ErrDocumentChildUnresolved) {
			t.Fatal("old parent did not observe the already-proven child exit")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("old parent did not Wait after recovery killed its owned child")
	}
	if err := RecoverDocumentStructureAttempt(t.Context(), control.TaskID, control.Attempt); err != nil {
		t.Fatal("attempt recovery after cleanup should be idempotent")
	}
}

func TestDocumentChildUnsafeCleanupKeepsFence(t *testing.T) {
	input, control := documentChildOwnedTestInput(t, "synthetic-protected-input")
	protected := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(protected, []byte("outside-must-stay"), 0600); err != nil {
		t.Fatal(err)
	}
	var observed DocumentChildOwnership
	_, err := RunDocumentStructureChild(t.Context(), input, control, func(owner DocumentChildOwnership) error {
		observed = owner
		if err := os.Remove(filepath.Join(owner.Directory, "input")); err != nil {
			return err
		}
		return os.Symlink(protected, filepath.Join(owner.Directory, "input"))
	})
	var failure *DocumentChildError
	if !errors.Is(err, ErrDocumentChildUnresolved) || !errors.As(err, &failure) || failure.Code != "cleanup_failed" {
		t.Fatal("unsafe cleanup was turned into terminal success/failure")
	}
	if raw, err := os.ReadFile(protected); err != nil || string(raw) != "outside-must-stay" {
		t.Fatal("cleanup followed a symlink outside its owned input")
	}
	// Restore only this unit test's exact synthetic file; no recursive deletion.
	if err := os.Remove(filepath.Join(observed.Directory, "input")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(observed.Directory, "input"), input, 0600); err != nil {
		t.Fatal(err)
	}
	if err := RecoverDocumentStructureChild(t.Context(), observed); err != nil {
		t.Fatal("explicitly repaired test-owned cleanup did not recover")
	}
}

func TestDocumentChildIncompleteAttemptMarkerBlocksNewExecution(t *testing.T) {
	input, control := documentChildOwnedTestInput(t, "staged-before-start")
	owner, file, err := documentChildPrepare(t.Context(), input, control)
	if err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	if err := RecoverDocumentStructureAttempt(t.Context(), control.TaskID, control.Attempt); !errors.Is(err, ErrDocumentChildUnresolved) {
		t.Fatal("live parent preparation was assumed to have finished")
	}
	_, err = RunDocumentStructureChild(t.Context(), input, control, func(DocumentChildOwnership) error { return nil })
	if !errors.Is(err, ErrDocumentChildUnresolved) {
		t.Fatal("existing unknown attempt was overwritten or retried")
	}
	if err := documentChildCleanup(owner); err != nil {
		t.Fatal("unit test could not clean its known not-started preparation")
	}
}

func TestDocumentChildStartupWrongTokenNeverAuthorizes(t *testing.T) {
	input, control := documentChildOwnedTestInput(t, "test:block")
	ctx, cancel := context.WithCancel(t.Context())
	started := make(chan DocumentChildOwnership, 1)
	release := make(chan struct{})
	var releaseOnce sync.Once
	done := make(chan error, 1)
	t.Cleanup(func() {
		cancel()
		releaseOnce.Do(func() { close(release) })
	})
	go func() {
		_, err := RunDocumentStructureChild(ctx, input, control, func(owner DocumentChildOwnership) error {
			started <- owner
			<-release // The correctly owned holder has not sent any parser control.
			return nil
		})
		done <- err
	}()
	var owner DocumentChildOwnership
	select {
	case owner = <-started:
	case err := <-done:
		t.Fatalf("owned holder did not reach startup gate: controlled_failure=%#v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("owned holder did not start")
	}
	// Exercise the exact production gate against a real, live same-PID/ticks/
	// PGID/exe child, changing only the expected token. A persistent wrong token
	// must never authorize the marker-write callback or the later onStarted.
	wrong := owner
	wrong.Token = strings.Repeat("b", 64)
	if wrong.Token == owner.Token {
		wrong.Token = strings.Repeat("c", 64)
	}
	called := false
	probe, err := documentChildAuthorizeStart(t.Context(), wrong, func() error { called = true; return nil })
	if err == nil || called || probe != "token_mismatch" {
		t.Fatalf("wrong token authorized startup: callback=%t probe=%q controlled_failure=%#v", called, probe, err)
	}
	if err := unix.Kill(owner.PID, 0); err != nil {
		t.Fatal("unproven startup gate signaled the held child")
	}
	t.Run("cancelled_context_never_authorizes", func(t *testing.T) {
		cancelled, stop := context.WithCancel(t.Context())
		stop()
		called := false
		_, err := documentChildAuthorizeStart(cancelled, owner, func() error { called = true; return nil })
		if called || !errors.Is(err, context.Canceled) {
			t.Fatal("already cancelled startup was authorized")
		}
	})
	cancel()
	releaseOnce.Do(func() { close(release) })
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) || errors.Is(err, ErrDocumentChildUnresolved) {
			t.Fatalf("correct holder did not stop after startup proof test: controlled_failure=%#v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("held child was not reaped")
	}
}
