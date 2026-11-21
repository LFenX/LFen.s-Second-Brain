//go:build integration && linux

package dataset

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"ragflow/internal/entity"
	"ragflow/internal/storage"
)

// Fixed predicate counterexamples only; never Root scopes or runtime admission.
func mysqlBaseGuardInputs() (mysqlBaseLease, mysqlBaseIdentity, mysqlBaseSeed, time.Time) {
	now := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	run := strings.Repeat("a", 16)
	lease := mysqlBaseLease{mysqlAccessLease: mysqlAccessLease{Purpose: "ROOT_ACCESS02_S1_MYSQL_CREATE_BASE", RunID: run, Schema: "lfen_s1_mysql_" + run, Address: "127.0.0.1:3306", ServerUUID: "synthetic-server", TriggerSHA: strings.Repeat("1", 64), NotBefore: now.Add(-time.Second), Expires: now.Add(3 * time.Minute), Dedicated: true, Primary: true, MySQL8: true, ResourcesVerified: true, ResourceManifestSHA: strings.Repeat("2", 64), TestELFSHA: strings.Repeat("3", 64), TestELFBytes: 1024, DictPath: "/synthetic/dict"}, MainSchema: "synthetic_main", Ownership: strings.Repeat("b", 32), IdentitySHA: strings.Repeat("4", 64), SeedSHA: strings.Repeat("5", 64), IdentityNotBefore: now.Add(-30 * time.Second), CatalogSHA: strings.Repeat("6", 64), RegistrationKey: "synthetic-registration", CommitKey: "synthetic-commit", BodySHA: lexicalSHA([]byte("synthetic\n")), BodyBytes: len("synthetic\n"), GrantExpires: now.Add(time.Hour)}
	identity := mysqlBaseIdentity{DomainID: strings.Repeat("c", 32), RunID: run, TargetTenant: strings.Repeat("d", 32), Attempts: 7, Bytes: 512, Origin: "actual-new-native-register", MainPersonalTenants: 2, IsolatedTargetTenants: 1}
	for i, p := range []*mysqlBaseSession{&identity.Owner, &identity.Recipient} {
		role := []string{"owner", "recipient"}[i]
		*p = mysqlBaseSession{Raw: strings.Repeat([]string{"7", "8"}[i], 32), IssuedAt: now.Add(-20 * time.Second).UnixMilli(), ExpiresAt: now.Add(24*time.Hour - 20*time.Second).UnixMilli(), Authorization: "synthetic-signed-" + role, ID: []string{identity.TargetTenant, strings.Repeat("e", 32)}[i], Email: "mysql-" + run + "-" + role + "@example.invalid", Nickname: "Synthetic " + run + " " + []string{"0", "1"}[i]}
	}
	seed := mysqlBaseSeed{Version: "mysql-provisioner-result/1", Phase: "seed", RunID: run, Schema: lease.Schema, Ownership: lease.Ownership, Completed: true, Calls: 6, Rows: 58, TriggerSHA: lease.TriggerSHA, CatalogSHA: lease.CatalogSHA}
	return lease, identity, seed, now
}

func TestKnowledgeAccessMySQLBaseDefaultOff(t *testing.T) {
	t.Run("before_any_private_input", func(t *testing.T) {
		t.Setenv("LFEN_ACCESS_MYSQL_BASE", "")
		t.Setenv("LFEN_ACCESS_MYSQL_BASE_RAM_PIPE", "/nonexistent-never-opened")
		TestKnowledgeAccessMySQLCreateBase(t)
		t.Fatal("default_off_did_not_skip")
	})
}

func TestKnowledgeAccessMySQLBaseLeaseGuards(t *testing.T) {
	lease, _, _, now := mysqlBaseGuardInputs()
	if validateMySQLBaseLease(lease, now, lease.Expires) != nil {
		t.Fatal("bounded_lease_rejected")
	}
	cases := []struct {
		name   string
		change func(*mysqlBaseLease)
	}{
		{"expired", func(l *mysqlBaseLease) { l.Expires = now.Add(-time.Second) }},
		{"future", func(l *mysqlBaseLease) { l.NotBefore = now.Add(time.Second) }},
		{"unbounded", func(l *mysqlBaseLease) { l.Expires = now.Add(11 * time.Minute) }},
		{"wrong_schema", func(l *mysqlBaseLease) { l.Schema = l.MainSchema }},
		{"other_run_schema", func(l *mysqlBaseLease) { l.Schema = "lfen_s1_mysql_" + strings.Repeat("f", 16) }},
		{"schema_suffix", func(l *mysqlBaseLease) { l.Schema += "_extra" }},
		{"wrong_purpose", func(l *mysqlBaseLease) { l.Purpose = "ROOT_ACCESS02_S1_MYSQL_PREPARE_FIXTURE" }},
		{"already_prepared", func(l *mysqlBaseLease) { l.Prepared = true }},
		{"old_elf", func(l *mysqlBaseLease) { l.TestELFSHA = mysqlBaseOldELFSHA }},
		{"old_fixture", func(l *mysqlBaseLease) { l.FixtureSHA = strings.Repeat("9", 64) }},
		{"resource_unverified", func(l *mysqlBaseLease) { l.ResourcesVerified = false }},
		{"body_over_cap", func(l *mysqlBaseLease) { l.BodyBytes = 8193 }},
		{"registration_key_over_cap", func(l *mysqlBaseLease) { l.RegistrationKey = strings.Repeat("a", 65) }},
		{"grant_outlives_native_session", func(l *mysqlBaseLease) { l.GrantExpires = now.Add(25 * time.Hour) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l, _, _, _ := mysqlBaseGuardInputs()
			tc.change(&l)
			if validateMySQLBaseLease(l, now, l.Expires) == nil {
				t.Fatal("invalid_lease_accepted")
			}
		})
	}
	if validateMySQLBaseLease(lease, now, lease.Expires.Add(time.Second)) == nil {
		t.Fatal("bootstrap_expiry_mismatch_accepted")
	}
}

func TestKnowledgeAccessMySQLBaseIdentityGuards(t *testing.T) {
	lease, identity, seed, now := mysqlBaseGuardInputs()
	if validateMySQLBaseIdentity(lease, identity, seed, now) != nil {
		t.Fatal("bounded_identity_rejected")
	}
	cases := []struct {
		name   string
		change func(*mysqlBaseIdentity, *mysqlBaseSeed)
	}{
		{"same_user", func(i *mysqlBaseIdentity, _ *mysqlBaseSeed) { i.Recipient.ID = i.Owner.ID }},
		{"same_token", func(i *mysqlBaseIdentity, _ *mysqlBaseSeed) { i.Recipient.Raw = i.Owner.Raw }},
		{"old_issuance", func(i *mysqlBaseIdentity, _ *mysqlBaseSeed) { i.Owner.IssuedAt = now.Add(-time.Hour).UnixMilli() }},
		{"invented_expiry", func(i *mysqlBaseIdentity, _ *mysqlBaseSeed) { i.Owner.ExpiresAt += 1000 }},
		{"wrong_tenant", func(i *mysqlBaseIdentity, _ *mysqlBaseSeed) { i.TargetTenant = i.Recipient.ID }},
		{"existing_origin", func(i *mysqlBaseIdentity, _ *mysqlBaseSeed) { i.Origin = "existing-login" }},
		{"incomplete_native_sequence", func(i *mysqlBaseIdentity, _ *mysqlBaseSeed) { i.Attempts = 6 }},
		{"different_seed_run", func(_ *mysqlBaseIdentity, s *mysqlBaseSeed) { s.RunID = strings.Repeat("f", 16) }},
		{"different_seed_owner", func(_ *mysqlBaseIdentity, s *mysqlBaseSeed) { s.Ownership = strings.Repeat("f", 32) }},
		{"different_catalog", func(_ *mysqlBaseIdentity, s *mysqlBaseSeed) { s.CatalogSHA = strings.Repeat("f", 64) }},
		{"seed_not_completed", func(_ *mysqlBaseIdentity, s *mysqlBaseSeed) { s.Completed = false }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l, i, s, at := mysqlBaseGuardInputs()
			tc.change(&i, &s)
			if validateMySQLBaseIdentity(l, i, s, at) == nil {
				t.Fatal("invalid_identity_accepted")
			}
		})
	}
}

func mysqlBaseGuardState(identity mysqlBaseIdentity) mysqlBaseSeedState {
	valid, super := "1", false
	state := mysqlBaseSeedState{Tenant: entity.Tenant{ID: identity.TargetTenant, Status: &valid}, KB: entity.Knowledgebase{ID: identity.DomainID, TenantID: identity.TargetTenant, CreatedBy: identity.Owner.ID, Status: &valid, Permission: "me", KnowledgeEnabled: true, KnowledgePolicyVersion: 1, KnowledgeCategory: "custom", ParserID: "naive"}}
	for i, session := range []mysqlBaseSession{identity.Owner, identity.Recipient} {
		raw := session.Raw
		state.Users = append(state.Users, entity.User{ID: session.ID, AccessToken: &raw, Email: session.Email, Nickname: session.Nickname, IsAuthenticated: "1", IsActive: "1", IsAnonymous: "0", IsSuperuser: &super, Status: &valid})
		state.Members = append(state.Members, entity.UserTenant{ID: strings.Repeat([]string{"1", "2"}[i], 32), UserID: session.ID, TenantID: identity.TargetTenant, Role: []string{"owner", "normal"}[i], InvitedBy: identity.Owner.ID, Status: &valid})
	}
	return state
}

func TestKnowledgeAccessMySQLBaseSeedStateGuards(t *testing.T) {
	_, identity, _, _ := mysqlBaseGuardInputs()
	if validateMySQLBaseSeedState(identity, mysqlBaseGuardState(identity)) != nil {
		t.Fatal("native_seed_snapshot_rejected")
	}
	cases := []struct {
		name   string
		change func(*mysqlBaseSeedState)
	}{
		{"session_changed", func(s *mysqlBaseSeedState) { raw := strings.Repeat("f", 32); s.Users[0].AccessToken = &raw }},
		{"superuser_unknown", func(s *mysqlBaseSeedState) { s.Users[0].IsSuperuser = nil }},
		{"superuser", func(s *mysqlBaseSeedState) { yes := true; s.Users[0].IsSuperuser = &yes }},
		{"inactive", func(s *mysqlBaseSeedState) { s.Users[0].IsActive = "0" }},
		{"duplicate_user", func(s *mysqlBaseSeedState) { s.Users[1] = s.Users[0] }},
		{"wrong_member_role", func(s *mysqlBaseSeedState) { s.Members[1].Role = "owner" }},
		{"wrong_member_tenant", func(s *mysqlBaseSeedState) { s.Members[1].TenantID = strings.Repeat("f", 32) }},
		{"duplicate_member", func(s *mysqlBaseSeedState) { s.Members[1] = s.Members[0] }},
		{"generation_changed", func(s *mysqlBaseSeedState) { s.KB.KnowledgePolicyVersion = 2 }},
		{"already_has_folder", func(s *mysqlBaseSeedState) { folder := strings.Repeat("f", 32); s.KB.KnowledgeFolderID = &folder }},
		{"wrong_owner", func(s *mysqlBaseSeedState) { s.KB.CreatedBy = identity.Recipient.ID }},
		{"nonempty_domain", func(s *mysqlBaseSeedState) { s.KB.DocNum = 1 }},
		{"embedding", func(s *mysqlBaseSeedState) { s.Members[0].EmbdID = "synthetic-model" }},
		{"generation", func(s *mysqlBaseSeedState) { s.Members[0].LLMID = "synthetic-model" }},
		{"rerank", func(s *mysqlBaseSeedState) { s.Members[0].RerankID = "synthetic-model" }},
		{"asr", func(s *mysqlBaseSeedState) { s.Members[0].ASRID = "synthetic-model" }},
		{"image_model", func(s *mysqlBaseSeedState) { s.Members[0].Img2TxtID = "synthetic-model" }},
		{"tts", func(s *mysqlBaseSeedState) { model := "synthetic-model"; s.Members[0].TTSID = &model }},
		{"ocr", func(s *mysqlBaseSeedState) { model := "synthetic-model"; s.Members[0].OCRID = &model }},
		{"pipeline", func(s *mysqlBaseSeedState) { id := strings.Repeat("f", 32); s.KB.PipelineID = &id }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := mysqlBaseGuardState(identity)
			tc.change(&state)
			if validateMySQLBaseSeedState(identity, state) == nil {
				t.Fatal("invalid_native_seed_accepted")
			}
		})
	}
}

func TestKnowledgeAccessMySQLBaseOutputBinding(t *testing.T) {
	_, identity, _, _ := mysqlBaseGuardInputs()
	body, folder := "synthetic\n", strings.Repeat("f", 32)
	base := mysqlPreparationBase{Owner: identity.Owner.ID, Recipient: identity.Recipient.ID, Tenant: identity.TargetTenant, Domain: identity.DomainID, Registration: strings.Repeat("1", 32), Import: strings.Repeat("2", 32), Root: strings.Repeat("3", 32), ReceiptSHA: strings.Repeat("4", 64), SourceCreated: true, Target: entity.KnowledgeAccessTarget{Source: entity.KnowledgeAccessSource{DomainID: identity.DomainID, SourceID: strings.Repeat("5", 32), SourceRevisionID: strings.Repeat("6", 32), SourceSHA256: lexicalSHA([]byte(body))}, Representation: entity.KnowledgeAccessRepresentation{Kind: "source_revision", ID: strings.Repeat("6", 32)}}}
	base.PolicyBaseline = mysqlPreparationPolicyBaseline{LocalKey: "synthetic-local", ContentKey: "synthetic-content", LocalRequestSHA: strings.Repeat("7", 64), LocalReceiptSHA: strings.Repeat("8", 64), ContentRequestSHA: strings.Repeat("9", 64), ContentReceiptSHA: strings.Repeat("a", 64), FoundationVersion: 1, LocalVersion: 1, ContentVersion: 2}
	base.GenerationBaseline = mysqlPreparationGenerationBaseline{TenantStatePresent: true, TenantEpoch: 7, SourceStateSHA256: strings.Repeat("b", 64)}
	base.PolicyBaseline.LocalOperationID = lexicalDigest("knowledge-local-operation/1", base.Owner, base.Domain, "policy", base.PolicyBaseline.LocalKey)
	base.PolicyBaseline.ContentOperationID = parsedOperationID(base.Owner, base.Domain, "parsed_policy", base.PolicyBaseline.ContentKey)
	store := storage.NewMemoryStorage()
	if store.Put(context.Background(), folder, ".objects/"+base.Target.Source.SourceSHA256, []byte(body)) != nil {
		t.Fatal("memory_object")
	}
	objects, err := mysqlBaseObjects(context.Background(), store)
	base.Objects = objects
	if err != nil || validateMySQLBaseOutput(base, folder, body) != nil {
		t.Fatal("actual_memory_inventory_rejected")
	}
	cases := []struct {
		name   string
		change func(*mysqlPreparationBase)
	}{
		{"wrong_target", func(b *mysqlPreparationBase) { b.Target.Source.DomainID = identity.Recipient.ID }},
		{"wrong_revision", func(b *mysqlPreparationBase) { b.Target.Representation.ID = strings.Repeat("f", 32) }},
		{"producer_injected", func(b *mysqlPreparationBase) {
			producer := strings.Repeat("a", 64)
			b.Target.Representation.ProducerSHA256 = &producer
		}},
		{"wrong_immutable_bucket", func(b *mysqlPreparationBase) { b.Objects[0].Bucket = identity.DomainID }},
		{"wrong_bytes", func(b *mysqlPreparationBase) { b.Objects[0].Body = "other" }},
		{"duplicate_object", func(b *mysqlPreparationBase) { b.Objects = append(b.Objects, b.Objects[0]) }},
		{"unbound_native_operation", func(b *mysqlPreparationBase) { b.PolicyBaseline.LocalOperationID = strings.Repeat("b", 64) }},
		{"wrong_content_generation", func(b *mysqlPreparationBase) { b.PolicyBaseline.ContentVersion++ }},
		{"missing_observed_epoch", func(b *mysqlPreparationBase) { b.GenerationBaseline.TenantStatePresent = false }},
		{"missing_source_generation", func(b *mysqlPreparationBase) { b.GenerationBaseline.SourceStateSHA256 = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			copy := base
			copy.Objects = append([]mysqlAccessObject(nil), base.Objects...)
			tc.change(&copy)
			if validateMySQLBaseOutput(copy, folder, body) == nil {
				t.Fatal("invalid_base_output_accepted")
			}
		})
	}
}

type mysqlAccessObservedStream struct {
	*os.File
	stats, reads, deadlines  int
	deadline                 time.Time
	afterStat, afterDeadline func()
}

func (s *mysqlAccessObservedStream) Stat() (os.FileInfo, error) {
	s.stats++
	info, err := s.File.Stat()
	if s.afterStat != nil {
		s.afterStat()
	}
	return info, err
}

func (s *mysqlAccessObservedStream) SetReadDeadline(deadline time.Time) error {
	s.deadlines++
	s.deadline = deadline
	err := s.File.SetReadDeadline(deadline)
	if s.afterDeadline != nil {
		s.afterDeadline()
	}
	return err
}

func (s *mysqlAccessObservedStream) Read(raw []byte) (int, error) {
	s.reads++
	return s.File.Read(raw)
}

func mysqlAccessGuardFIFO(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if os.Chmod(dir, 0700) != nil {
		t.Fatal("private_parent_mode")
	}
	name := filepath.Join(dir, "scope.pipe")
	if syscall.Mkfifo(name, 0600) != nil {
		t.Fatal("private_fifo")
	}
	return name
}

func TestKnowledgeAccessMySQLRAMAbsoluteExpiry(t *testing.T) {
	hash := strings.Repeat("a", 64)
	t.Run("relative_and_absolute_caps", func(t *testing.T) {
		now := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
		for _, remaining := range []time.Duration{2 * time.Second, 10 * time.Second} {
			deadline, err := mysqlAccessRAMDeadline(now, now.Add(remaining))
			want := now.Add(5 * time.Second)
			if remaining < 5*time.Second {
				want = now.Add(remaining)
			}
			if err != nil || !deadline.Equal(want) {
				t.Fatal("wrong_absolute_or_relative_cap")
			}
		}
		if _, err := mysqlAccessRAMDeadline(now, now); err == nil {
			t.Fatal("expiry_equality_admitted")
		}
	})
	t.Run("bootstrap_then_delay_past_expiry", func(t *testing.T) {
		lease, _, _, clock := mysqlBaseGuardInputs()
		if validateMySQLBaseLease(lease, clock, lease.Expires) != nil || !lease.Expires.After(clock.Add(6*time.Second)) {
			t.Fatal("bootstrap_not_valid")
		}
		metadata, opens := 0, 0
		reader := mysqlAccessSystemRAM()
		reader.now = func() time.Time { return clock }
		reader.lstat = func(name string) (os.FileInfo, error) { metadata++; return os.Lstat(name) }
		reader.open = func(name string, flag int, mode os.FileMode) (mysqlAccessRAMStream, error) {
			opens++
			return os.OpenFile(name, flag, mode)
		}
		clock = lease.Expires.Add(time.Nanosecond)
		if raw, err := reader.read("/synthetic/private/scope.pipe", hash, lease.Expires); err == nil || len(raw) != 0 || metadata != 0 || opens != 0 {
			t.Fatal("expired_bootstrap_performed_io")
		}
	})
	for _, stage := range []string{"fifo_lstat", "parent_lstat", "open", "stat", "deadline"} {
		t.Run("delay_after_"+stage, func(t *testing.T) {
			name := mysqlAccessGuardFIFO(t)
			clock, expires := time.Now(), time.Now().Add(time.Second)
			metadata, opens := 0, 0
			var observed *mysqlAccessObservedStream
			reader := mysqlAccessSystemRAM()
			reader.now = func() time.Time { return clock }
			reader.lstat = func(name string) (os.FileInfo, error) {
				metadata++
				info, err := os.Lstat(name)
				if stage == "fifo_lstat" && metadata == 1 || stage == "parent_lstat" && metadata == 2 {
					clock = expires.Add(time.Nanosecond)
				}
				return info, err
			}
			reader.open = func(name string, flag int, mode os.FileMode) (mysqlAccessRAMStream, error) {
				opens++
				if flag&syscall.O_NONBLOCK == 0 {
					t.Fatal("blocking_fifo_open")
				}
				file, err := os.OpenFile(name, flag, mode)
				if err != nil {
					return nil, err
				}
				observed = &mysqlAccessObservedStream{File: file}
				switch stage {
				case "open":
					clock = expires.Add(time.Nanosecond)
				case "stat":
					observed.afterStat = func() { clock = expires.Add(time.Nanosecond) }
				case "deadline":
					observed.afterDeadline = func() { clock = expires.Add(time.Nanosecond) }
				}
				return observed, nil
			}
			if raw, err := reader.read(name, hash, expires); err == nil || len(raw) != 0 {
				t.Fatal("io_boundary_delay_accepted")
			}
			if (stage == "fifo_lstat" || stage == "parent_lstat") && opens != 0 {
				t.Fatal("expired_metadata_opened_fifo")
			}
			if stage == "fifo_lstat" && metadata != 1 || stage == "parent_lstat" && metadata != 2 || stage != "fifo_lstat" && stage != "parent_lstat" && (opens != 1 || observed == nil) {
				t.Fatal("requested_io_boundary_not_reached")
			}
			if observed != nil && (observed.reads != 0 || observed.deadline.After(expires)) {
				t.Fatal("expired_boundary_read_or_extended_deadline")
			}
			if stage == "open" && observed.stats != 0 || stage == "stat" && observed.deadlines != 0 {
				t.Fatal("expired_boundary_advanced_io")
			}
			if stage == "stat" && observed.stats != 1 || stage == "deadline" && observed.deadlines != 1 {
				t.Fatal("requested_stat_or_deadline_boundary_not_reached")
			}
		})
	}
	t.Run("actual_fifo_frame_clipped_to_expiry", func(t *testing.T) {
		name := mysqlAccessGuardFIFO(t)
		writer, err := os.OpenFile(name, os.O_RDWR|syscall.O_NONBLOCK, 0)
		if err != nil {
			t.Fatal("nonblocking_ready_writer")
		}
		defer writer.Close()
		body := []byte("synthetic-private-frame")
		if n, err := writer.Write(body); err != nil || n != len(body) {
			t.Fatal("guard_frame_write")
		}
		var observed *mysqlAccessObservedStream
		reader := mysqlAccessSystemRAM()
		expires := time.Now().Add(time.Second)
		reader.open = func(name string, flag int, mode os.FileMode) (mysqlAccessRAMStream, error) {
			file, err := os.OpenFile(name, flag, mode)
			if err != nil {
				return nil, err
			}
			observed = &mysqlAccessObservedStream{File: file}
			writer.Close()
			return observed, nil
		}
		raw, err := reader.read(name, hash, expires)
		if err != nil || string(raw) != string(body) || observed == nil || observed.reads == 0 || !observed.deadline.Equal(expires) {
			t.Fatal("actual_fifo_frame_or_absolute_deadline")
		}
		clear(raw)
	})
	t.Run("actual_read_expires_without_new_five_seconds", func(t *testing.T) {
		name := mysqlAccessGuardFIFO(t)
		writer, err := os.OpenFile(name, os.O_RDWR|syscall.O_NONBLOCK, 0)
		if err != nil {
			t.Fatal("nonblocking_ready_writer")
		}
		defer writer.Close()
		reader := mysqlAccessSystemRAM()
		var observed *mysqlAccessObservedStream
		reader.open = func(name string, flag int, mode os.FileMode) (mysqlAccessRAMStream, error) {
			file, err := os.OpenFile(name, flag, mode)
			if err != nil {
				return nil, err
			}
			observed = &mysqlAccessObservedStream{File: file}
			return observed, nil
		}
		start := time.Now()
		expires := start.Add(80 * time.Millisecond)
		if raw, err := reader.read(name, hash, expires); err == nil || len(raw) != 0 || observed == nil || observed.reads == 0 || observed.deadlines != 1 || !observed.deadline.Equal(expires) || time.Since(start) < 25*time.Millisecond || time.Since(start) > 2*time.Second {
			t.Fatal("absolute_read_deadline_not_enforced")
		}
	})
}
