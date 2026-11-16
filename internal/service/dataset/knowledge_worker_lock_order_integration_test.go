//go:build integration && linux

package dataset

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	gormMySQL "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"ragflow/internal/dao"
)

type workerLockInput struct {
	Purpose    string    `json:"purpose"`
	RunID      string    `json:"run_id"`
	ExpiresAt  time.Time `json:"expires_at"`
	Schema     string    `json:"schema"`
	Address    string    `json:"address"`
	ServerUUID string    `json:"server_uuid"`
	DSN        string    `json:"dsn"`
	UserID     string    `json:"user_id"`
	DomainID   string    `json:"domain_id"`
}

// The private pipe identifies one pre-existing synthetic domain. Both competing
// transactions roll back; this test neither publishes content nor changes grants.
func TestKnowledgeWorkerMySQLAccessEpochLockOrder(t *testing.T) {
	stat, err := os.Stdin.Stat()
	if err != nil || stat.Mode()&os.ModeNamedPipe == 0 {
		t.Fatal("worker integration requires a private input pipe")
	}
	raw, err := io.ReadAll(io.LimitReader(os.Stdin, 65537))
	if err != nil || len(raw) > 65536 {
		t.Fatal("worker integration input refused")
	}
	defer clear(raw)
	var input workerLockInput
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || input.Purpose != "root-synthetic-worker-lock-order" || !isKnowledgeID(input.RunID, 32) || !isKnowledgeID(input.UserID, 32) || !isKnowledgeID(input.DomainID, 32) || input.Schema != "second_brain_s1_acceptance_20261116" || !time.Now().Before(input.ExpiresAt) || input.ExpiresAt.After(time.Now().Add(3*time.Minute)) {
		t.Fatal("worker integration lease refused")
	}
	defer func() { input.DSN = "" }()
	parsed, err := mysqlDriver.ParseDSN(input.DSN)
	host, port, endpointErr := net.SplitHostPort(input.Address)
	if err != nil || endpointErr != nil || net.ParseIP(host) == nil || port != "3306" || parsed.Net != "tcp" || parsed.Addr != input.Address || parsed.DBName != input.Schema || !parsed.ParseTime || parsed.InterpolateParams || parsed.MultiStatements || parsed.TLSConfig != "" {
		t.Fatal("worker integration database endpoint refused")
	}
	parsed.Timeout, parsed.ReadTimeout, parsed.WriteTimeout, parsed.Loc = 5*time.Second, 5*time.Second, 5*time.Second, time.UTC
	pool, err := sql.Open("mysql", parsed.FormatDSN())
	if err != nil {
		t.Fatal("worker integration pool failed")
	}
	defer pool.Close()
	pool.SetMaxOpenConns(4)
	pool.SetMaxIdleConns(4)
	ctx, cancel := context.WithDeadline(t.Context(), input.ExpiresAt)
	defer cancel()
	var schema, server, version string
	var readOnly int
	if pool.QueryRowContext(ctx, "SELECT DATABASE(),@@server_uuid,@@version,@@read_only").Scan(&schema, &server, &version, &readOnly) != nil || schema != input.Schema || server != input.ServerUUID || !strings.HasPrefix(version, "8.") || readOnly != 0 {
		t.Fatal("worker integration primary identity failed")
	}
	var tenant, email string
	if pool.QueryRowContext(ctx, "SELECT kb.tenant_id,u.email FROM knowledgebase kb JOIN user u ON u.id=kb.created_by WHERE kb.id=? AND kb.created_by=? AND kb.knowledge_enabled=TRUE", input.DomainID, input.UserID).Scan(&tenant, &email) != nil || !isKnowledgeID(tenant, 32) || !strings.HasSuffix(email, "@fixtures.invalid") {
		t.Fatal("worker integration synthetic ownership failed")
	}
	var epochs int
	if pool.QueryRowContext(ctx, "SELECT COUNT(*) FROM knowledge_access_tenant_state WHERE tenant_id=?", tenant).Scan(&epochs) != nil || epochs != 1 {
		t.Fatal("worker integration requires existing epoch row")
	}
	db, err := gorm.Open(gormMySQL.New(gormMySQL.Config{Conn: pool, SkipInitializeWithVersion: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableAutomaticPing: true})
	if err != nil {
		t.Fatal("worker integration GORM failed")
	}
	previous := dao.DB
	dao.DB = db
	defer func() { dao.DB = previous }()
	rollback := errors.New("owned worker probe rollback")
	for _, test := range []struct {
		name string
		run  func(context.Context, string, func(*gorm.DB) error) error
	}{{"structure", structureInternalTransaction}, {"batch", batchInternalTransaction}} {
		t.Run(test.name, func(t *testing.T) {
			work, stop := context.WithTimeout(ctx, 8*time.Second)
			defer stop()
			blocker, err := pool.BeginTx(work, nil)
			if err != nil {
				t.Fatal("worker integration blocker failed")
			}
			defer blocker.Rollback()
			var epoch int64
			if blocker.QueryRowContext(work, "SELECT epoch FROM knowledge_access_tenant_state WHERE tenant_id=? FOR UPDATE", tenant).Scan(&epoch) != nil {
				t.Fatal("worker integration epoch barrier failed")
			}
			reached := make(chan struct{}, 1)
			const callback = "test:worker_epoch_barrier"
			if db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Table == "knowledge_access_tenant_state" {
					select {
					case reached <- struct{}{}:
					default:
					}
				}
			}) != nil {
				t.Fatal("worker integration callback failed")
			}
			defer db.Callback().Create().Remove(callback)
			done := make(chan error, 1)
			go func() {
				done <- test.run(work, input.DomainID, func(tx *gorm.DB) error {
					if _, err := lockKnowledgeImportDomain(work, tx, input.UserID, input.DomainID); err != nil {
						return err
					}
					return rollback
				})
			}()
			select {
			case <-reached:
			case <-work.Done():
				_ = blocker.Rollback()
				<-done
				t.Fatal("worker integration did not reach epoch barrier")
			}
			var domain string
			lockErr := blocker.QueryRowContext(work, "SELECT id FROM knowledgebase WHERE id=? FOR UPDATE NOWAIT", input.DomainID).Scan(&domain)
			_ = blocker.Rollback()
			workerErr := <-done
			if lockErr != nil || domain != input.DomainID {
				t.Fatal("worker held domain while waiting for access epoch")
			}
			if !errors.Is(workerErr, rollback) {
				t.Fatal("worker did not complete under the shared lock order")
			}
		})
	}
	if !t.Failed() {
		t.Log(`{"state":"Passed_RealMySQLWorkerEpochLockOrder","workers":2,"barriers":2,"rollbackOnly":true,"modelCalls":0,"grantChanges":0}`)
	}
}
