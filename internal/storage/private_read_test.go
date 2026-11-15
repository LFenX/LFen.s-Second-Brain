package storage

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"ragflow/internal/common"
)

func TestKnowledgeAccessPrivateStorageFailuresDoNotLogDetails(t *testing.T) {
	if err := common.InitLogger("debug", common.FileOutput{}, "access-storage-test"); err != nil {
		t.Fatal(err)
	}
	core, logs := observer.New(zap.DebugLevel)
	old, oldSugar := common.Logger, common.Sugar
	common.Logger = zap.New(core)
	common.Sugar = common.Logger.Sugar()
	t.Cleanup(func() { common.Logger, common.Sugar = old, oldSugar })
	for _, backend := range []string{"minio", "s3", "oss"} {
		t.Run(backend, func(t *testing.T) {
			calls := 0
			failure := errors.New("synthetic-private-backend-error")
			var store Storage
			if backend == "minio" {
				store = newMinioStreamStorage(t, func(*http.Request) (*http.Response, error) { calls++; return nil, failure })
			} else {
				base := newS3TestStorage("us-east-1", func(*http.Request) (*http.Response, error) { calls++; return nil, failure })
				if backend == "s3" {
					store = base
				} else {
					store = &OSSStorage{client: base.client}
				}
			}
			logs.TakeAll()
			started := time.Now()
			_, err := store.Get(WithPrivateRead(t.Context()), "synthetic-private-bucket", "synthetic-private-key")
			if err == nil || strings.Contains(err.Error(), "synthetic-private") || logs.Len() != 0 || calls != 2 || time.Since(started) > 5*time.Second {
				t.Fatalf("private failure leaked or retried out of bound: calls=%d logs=%d", calls, logs.Len())
			}
		})
	}
	// The ordinary native MinIO path keeps its existing diagnostic behavior.
	ordinary := newMinioStreamStorage(t, func(*http.Request) (*http.Response, error) { return nil, errors.New("synthetic-ordinary-error") })
	logs.TakeAll()
	_, _ = ordinary.Get(t.Context(), "synthetic-native-bucket", "synthetic-native-key")
	if logs.Len() == 0 {
		t.Fatal("ordinary logging was globally disabled")
	}
}

func TestKnowledgeAccessPrivateStorageCancellationAndSuccess(t *testing.T) {
	if err := common.InitLogger("debug", common.FileOutput{}, "access-storage-test"); err != nil {
		t.Fatal(err)
	}
	core, logs := observer.New(zap.DebugLevel)
	old, oldSugar := common.Logger, common.Sugar
	common.Logger = zap.New(core)
	common.Sugar = common.Logger.Sugar()
	t.Cleanup(func() { common.Logger, common.Sugar = old, oldSugar })
	for _, backend := range []string{"minio", "s3", "oss"} {
		t.Run(backend, func(t *testing.T) {
			calls := 0
			ctx, cancel := context.WithCancel(t.Context())
			var store Storage
			if backend == "minio" {
				store = newMinioStreamStorage(t, func(*http.Request) (*http.Response, error) {
					calls++
					cancel()
					return nil, errors.New("synthetic-cancel-error")
				})
			} else {
				base := newS3TestStorage("us-east-1", func(*http.Request) (*http.Response, error) {
					calls++
					cancel()
					return nil, errors.New("synthetic-cancel-error")
				})
				if backend == "s3" {
					store = base
				} else {
					store = &OSSStorage{client: base.client}
				}
			}
			started := time.Now()
			_, err := store.Get(WithPrivateRead(ctx), "synthetic-cancel-bucket", "synthetic-cancel-key")
			cancel()
			if !errors.Is(err, context.Canceled) || calls > 1 || time.Since(started) > time.Second || logs.Len() != 0 {
				t.Fatalf("private cancellation not bounded: calls=%d logs=%d", calls, logs.Len())
			}
		})
	}
	base := newS3TestStorage("us-east-1", func(*http.Request) (*http.Response, error) { return s3Response(200, "synthetic exact bytes"), nil })
	for _, ctx := range []context.Context{t.Context(), WithPrivateRead(t.Context())} {
		body, err := base.Get(ctx, "synthetic-success-bucket", "synthetic-success-key")
		if err != nil || string(body) != "synthetic exact bytes" {
			t.Fatal("read semantics changed")
		}
	}
	if privateRead(context.WithValue(t.Context(), "private_read", true)) {
		t.Fatal("wire-like string key selected private mode")
	}
}
