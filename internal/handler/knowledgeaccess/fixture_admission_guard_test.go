//go:build integration && linux

package knowledgeaccess

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"ragflow/internal/entity"
)

// Artificial predicate inputs for offline tests; they are never Root artifacts
// or a runnable fixture scope, and no test supplies the runtime environment gate.
func fixtureGuardScope(now time.Time) fixtureLease {
	sourceSHA := strings.Repeat("a", 64)
	recipeSHA := strings.Repeat("b", 64)
	scope := fixtureLease{Purpose: "ROOT_ACCESS02_S1_SYNTHETIC_FIXTURE", RunID: strings.Repeat("1", 32), Origin: "http://127.0.0.1:1", NotBefore: now.Add(-time.Second), Expires: now.Add(time.Minute),
		Operator: true, SourceSetSHA: sourceSHA, OperatorRecipeSHA: recipeSHA, scopeSHA: strings.Repeat("c", 64)}
	scope.Synthetic = true
	scope.Prepare = true
	scope.Accounts = true
	scope.Foundation = true
	scope.Handoff = true
	scope.APIELF = fixtureAPIELF
	scope.MaterialsSHA = fixtureMaterialManifestSHA
	scope.RuntimeReceiptSHA = strings.Repeat("9", 64)
	scope.Safety = fixtureSafety{RegistrationEnabled: true, SMTPDisabledProven: true, GlobalDefaultsEmpty: true, OCRDisabled: true, ModelsDisabled: true, SyncDisabled: true, WritersBound: true, SecretLoggingOff: true}
	review := fixtureEvidenceReceipt{Kind: "independent_source_review", Artifact: fixtureArtifact{SHA: strings.Repeat("d", 64), Bytes: 100}, Actor: "review-B", SourceSetSHA: sourceSHA, SourceFiles: 21, Finished: now.Add(-time.Minute), Result: "approved"}
	compile := review
	compile.Kind = "linux_integration_compile"
	compile.Actor = "compile-C"
	compile.Artifact.SHA = strings.Repeat("e", 64)
	compile.Result = "passed"
	compile.GOOS = "linux"
	compile.BuildTag = "integration"
	scope.admission = fixtureAdmission{Version: "root-fixture-admission/2", RootID: "01a1a9a5-c724-7f52-8cbb-16d19d982ba4", RunID: scope.RunID, Origin: scope.Origin, NotBefore: scope.NotBefore, Expires: scope.Expires,
		Issued: now.Add(-time.Millisecond), ReferenceSHA: "ab12875dfc1e0385eb1df1c6b474b4439d8d0e811f57ccc1259497055ff1001e", SourceSetSHA: sourceSHA, RecipeSHA: recipeSHA, APIELF: fixtureAPIELF, RootVerified: true, Review: review, Compile: compile}
	for i, phase := range []string{"r1_root", "a_r2", "canonical_pre", "canonical_post"} {
		cap := fixtureOperatorCapability{ID: strings.Repeat(string(rune('2'+i)), 32), Name: "fixture_" + phase, Phase: phase, Driver: fixtureArtifact{SHA: strings.Repeat("f", 64), Bytes: 100}, NotBefore: scope.NotBefore, Expires: scope.Expires}
		switch phase {
		case "r1_root":
			cap.MaxHTTP = 4
			cap.MaxHTTPBytes = 1 << 20
			cap.MaxFileWrites = 2
		case "a_r2":
			cap.MaxHTTP = 4
			cap.MaxHTTPBytes = 1 << 20
			cap.MaxSQLSelect = 1
			cap.MaxSQLRows = 2
			cap.MaxCASRows = 2
			cap.MaxObjectPUT = 1
		default:
			cap.MaxSQLSelect = 16
			cap.MaxSQLRows = 128
		}
		scope.admission.Operators = append(scope.admission.Operators, cap)
	}
	return scope
}

func TestKnowledgeAccessFixtureAdmissionGuards(t *testing.T) {
	now := time.Now().UTC()
	valid := fixtureGuardScope(now)
	if gaps := fixturePreparationGaps(valid, valid.SourceSetSHA, now); len(gaps) != 0 {
		t.Fatal("bounded_admission_has_no_satisfiable_path")
	}
	cases := []struct {
		name   string
		change func(*fixtureLease)
	}{
		{"unverified", func(s *fixtureLease) { s.admission.RootVerified = false }},
		{"different_source", func(s *fixtureLease) { s.admission.Compile.SourceSetSHA = strings.Repeat("9", 64) }},
		{"missing_compile_receipt", func(s *fixtureLease) { s.admission.Compile.Artifact.SHA = strings.Repeat("0", 64) }},
		{"same_actor", func(s *fixtureLease) { s.admission.Compile.Actor = s.admission.Review.Actor }},
		{"wrong_platform", func(s *fixtureLease) { s.admission.Compile.GOOS = "windows" }},
		{"wrong_source_count", func(s *fixtureLease) { s.admission.Compile.SourceFiles = 19 }},
		{"expired", func(s *fixtureLease) { s.Expires = now; s.admission.Expires = now }},
		{"missing_operator", func(s *fixtureLease) { s.admission.Operators = s.admission.Operators[:3] }},
		{"operator_expiry_extended", func(s *fixtureLease) { s.admission.Operators[0].Expires = s.Expires.Add(time.Second) }},
		{"third_cas_row", func(s *fixtureLease) { s.admission.Operators[1].MaxCASRows = 3 }},
		{"capture_http", func(s *fixtureLease) { s.admission.Operators[2].MaxHTTP = 1 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := fixtureGuardScope(now)
			tc.change(&s)
			if len(fixturePreparationGaps(s, s.SourceSetSHA, now)) == 0 {
				t.Fatal("invalid_admission_accepted")
			}
		})
	}
}

func TestKnowledgeAccessFixturePrepaidOperatorFailure(t *testing.T) {
	for _, phase := range []string{"r1_root", "a_r2"} {
		t.Run(phase, func(t *testing.T) {
			s := fixtureGuardScope(time.Now())
			s.OutputPipe = filepath.Join(t.TempDir(), "not-a-fifo")
			client := newFixtureHTTP(s)
			defer client.close()
			domain, owner, source := strings.Repeat("6", 32), strings.Repeat("7", 32), ""
			if phase == "a_r2" {
				source = strings.Repeat("8", 32)
				raw, err := fixtureMaterial("synthetic-A.csv")
				if err != nil {
					t.Fatal("material")
				}
				client.parsedBindings[source] = fixtureParsedBinding{Source: entity.KnowledgeContentSourceRef{DomainID: domain, SourceID: source, RevisionID: strings.Repeat("9", 32), SHA256: digest(raw)}, NativeFileID: strings.Repeat("a", 32), Bytes: 36}
			}
			if _, err := client.operatorStage(phase, domain, owner, source); err == nil {
				t.Fatal("non_fifo_accepted")
			}
			if client.attempts != 4 || client.bodyBytes != 1<<20 || !client.stopped {
				t.Fatal("failed_operator_allocation_refunded")
			}
		})
	}
	for _, attempts := range []int{117, 120} {
		client := newFixtureHTTP(fixtureGuardScope(time.Now()))
		client.attempts = attempts
		if client.reserveOperator() == nil || client.attempts != attempts || client.bodyBytes != 0 {
			client.close()
			t.Fatal("external_capacity_not_checked_before_event")
		}
		client.close()
	}
}

func fixtureGuardWriter(ctx context.Context, name string, wire []byte) <-chan error {
	done := make(chan error, 1)
	go func() {
		fd := -1
		for ctx.Err() == nil {
			value, err := syscall.Open(name, syscall.O_WRONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
			if err == nil {
				fd = value
				break
			}
			if err != syscall.ENXIO {
				done <- err
				return
			}
			if fixturePause(ctx, time.Millisecond) != nil {
				done <- errFixture
				return
			}
		}
		if fd < 0 {
			done <- errFixture
			return
		}
		defer syscall.Close(fd)
		n, err := syscall.Write(fd, wire)
		if err == nil && n != len(wire) {
			err = errFixture
		}
		done <- err
	}()
	return done
}

func fixtureGuardFIFO(t *testing.T, leaf string) string {
	t.Helper()
	directory := t.TempDir()
	if os.Chmod(directory, 0700) != nil {
		t.Fatal("private_fifo_parent")
	}
	name := filepath.Join(directory, leaf)
	if syscall.Mkfifo(name, 0600) != nil {
		t.Fatal("fifo")
	}
	if _, err := fixtureFIFO(name); err != nil {
		t.Fatal("private_fifo_fixture")
	}
	return name
}

func TestKnowledgeAccessFixtureRAMFailureChargingAndExpiry(t *testing.T) {
	for index, limit := range []int{3, 32} {
		t.Run([]string{"overflow", "malformed"}[index], func(t *testing.T) {
			name := fixtureGuardFIFO(t, "ack")
			client := newFixtureHTTP(fixtureGuardScope(time.Now()))
			client.scope.OperatorAckPipe = name
			defer client.close()
			done := fixtureGuardWriter(client.ctx, name, []byte("oops"))
			raw, err := client.readOperatorRAM(limit)
			client.cancel()
			<-done
			if limit == 3 && err == nil {
				t.Fatal("overflow_accepted")
			}
			if limit == 32 && (err != nil || decodeClosed(raw, new(fixtureOperatorAck)) == nil) {
				t.Fatal("malformed_ack_predicate")
			}
			if client.bodyBytes != 4 {
				t.Fatal("failed_ack_bytes_not_charged")
			}
		})
	}
	name := fixtureGuardFIFO(t, "idle")
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(25*time.Millisecond))
	defer cancel()
	started := time.Now()
	if _, err := fixtureReadRAM(ctx, name, 32, nil); err == nil {
		t.Fatal("idle_pipe_accepted")
	}
	if time.Since(started) > time.Second {
		t.Fatal("absolute_expiry_extended_by_fixed_wait")
	}
	expired, stop := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer stop()
	if fd, err := fixtureOpenRAM(expired, name, false); err == nil {
		syscall.Close(fd)
		t.Fatal("open_after_expiry")
	}
}
