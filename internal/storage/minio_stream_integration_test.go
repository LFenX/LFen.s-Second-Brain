//go:build integration

// Copyright 2026 LFen's Second Brain contributors.
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"ragflow/internal/common"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type minioStreamIntegrationTransport struct {
	base    http.RoundTripper
	after   func(*http.Request) error
	failure error
}

func (r *minioStreamIntegrationTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	response, err := r.base.RoundTrip(req)
	if err == nil && r.after != nil {
		if hookErr := r.after(req); hookErr != nil {
			_ = response.Body.Close()
			r.failure = hookErr
			return nil, hookErr
		}
	}
	return response, err
}

func TestMinioStreamIntegration(t *testing.T) {
	raw := os.Getenv("LFEN_TEST_MINIO_CONFIG")
	if raw == "" {
		t.Skip("integration requires explicit LFEN_TEST_MINIO_CONFIG")
	}
	var config struct {
		Endpoint  string `json:"endpoint"`
		AccessKey string `json:"access_key"`
		SecretKey string `json:"secret_key"`
		Secure    bool   `json:"secure"`
	}
	if err := json.Unmarshal([]byte(raw), &config); err != nil || config.Endpoint == "" || config.AccessKey == "" || config.SecretKey == "" {
		t.Fatal("invalid explicit integration configuration")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	t.Cleanup(transport.CloseIdleConnections)
	newClient := func(rt http.RoundTripper) *minio.Client {
		client, err := minio.New(config.Endpoint, &minio.Options{
			Region: "us-east-1", Secure: config.Secure, BucketLookup: minio.BucketLookupPath,
			Creds:     credentials.NewStaticV4(config.AccessKey, config.SecretKey, ""),
			Transport: rt, MaxRetries: 1,
		})
		if err != nil {
			t.Fatal("could not configure integration client")
		}
		return client
	}
	client := newClient(transport)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	bucket := "lfen-r4-stream-" + common.GenerateUUID()
	if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: "us-east-1"}); err != nil {
		t.Fatalf("create isolated synthetic bucket: %v", err)
	}
	// Fixtures are retained in this unique bucket. Do not remove application
	// buckets, history or objects as a side effect of an integration test.
	t.Logf("retained_test_bucket=%s", bucket)
	store := &MinioStorage{client: client, bucket: bucket, prefixPath: "stage2r4"}
	const domain = "synthetic-domain"

	t.Run("capacity", func(t *testing.T) {
		for _, size := range []int64{3 << 20, 32 << 20, 64 << 20} {
			for run := 1; run <= 3; run++ {
				t.Run(fmt.Sprintf("%d/%d", size, run), func(t *testing.T) {
					hash := sha256Pattern(t, size)
					key := fmt.Sprintf("capacity/%d/%d.bin", size, run)
					source := &minioPatternReader{remaining: size}
					var before, after runtime.MemStats
					runtime.ReadMemStats(&before)
					start := time.Now()
					info, err := store.putStream(ctx, domain, key, source, size, "application/octet-stream", hash)
					putElapsed := time.Since(start)
					runtime.ReadMemStats(&after)
					if err != nil || info.Size != size || source.readBytes != size {
						t.Fatalf("synthetic PUT: size=%d read=%d err=%v", info.Size, source.readBytes, err)
					}
					putAllocation := after.TotalAlloc - before.TotalAlloc
					runtime.ReadMemStats(&before)
					start = time.Now()
					body, err := store.openStream(ctx, domain, key, size, hash, 0, size)
					if err != nil {
						t.Fatal(err)
					}
					actual := sha256.New()
					n, copyErr := io.CopyBuffer(actual, body, make([]byte, 64<<10))
					closeErr := body.Close()
					getElapsed := time.Since(start)
					runtime.ReadMemStats(&after)
					if copyErr != nil || closeErr != nil || n != size || hex.EncodeToString(actual.Sum(nil)) != hash {
						t.Fatalf("synthetic GET changed bytes: size=%d copy=%v close=%v", n, copyErr, closeErr)
					}
					getAllocation := after.TotalAlloc - before.TotalAlloc
					if putAllocation > 8<<20 || getAllocation > 8<<20 || source.maxBuffer > 1<<20 {
						t.Fatalf("stream allocation exceeded budget: put=%d get=%d max_read=%d", putAllocation, getAllocation, source.maxBuffer)
					}
					t.Logf("measurement size=%d run=%d put_ns=%d get_ns=%d put_alloc=%d get_alloc=%d max_read=%d", size, run, putElapsed.Nanoseconds(), getElapsed.Nanoseconds(), putAllocation, getAllocation, source.maxBuffer)
				})
			}
		}
	})

	content := make([]byte, 192<<10)
	for i := range content {
		content[i] = byte(i % 251)
	}
	hash := streamDigest(content)
	if _, err := store.putStream(ctx, domain, "ranges.bin", bytes.NewReader(content), int64(len(content)), "application/octet-stream", hash); err != nil {
		t.Fatal(err)
	}
	t.Run("ranges", func(t *testing.T) {
		for _, test := range []struct {
			name   string
			offset int64
			length int64
		}{{"first", 0, 1}, {"across_buffer", 65530, 31}, {"tail", int64(len(content) - 17), 17}, {"empty_at_end", int64(len(content)), 0}} {
			t.Run(test.name, func(t *testing.T) {
				body, err := store.openStream(ctx, domain, "ranges.bin", int64(len(content)), hash, test.offset, test.length)
				if err != nil {
					t.Fatal(err)
				}
				actual, readErr := io.ReadAll(body)
				closeErr := body.Close()
				if readErr != nil || closeErr != nil || !bytes.Equal(actual, content[test.offset:test.offset+test.length]) {
					t.Fatalf("fixed byte range changed: read=%v close=%v", readErr, closeErr)
				}
			})
		}
	})
	t.Run("empty_object", func(t *testing.T) {
		if _, err := store.putStream(ctx, domain, "empty.bin", strings.NewReader(""), 0, "application/octet-stream", streamDigest(nil)); err != nil {
			t.Fatal(err)
		}
		body, err := store.openStream(ctx, domain, "empty.bin", 0, streamDigest(nil), 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		data, readErr := io.ReadAll(body)
		if err := body.Close(); err != nil || readErr != nil || len(data) != 0 {
			t.Fatalf("empty object: read=%v close=%v", readErr, err)
		}
	})
	t.Run("metadata_rejection", func(t *testing.T) {
		for _, mismatch := range []struct {
			size int64
			hash string
		}{{int64(len(content)) + 1, hash}, {int64(len(content)), strings.Repeat("a", 64)}} {
			body, err := store.openStream(ctx, domain, "ranges.bin", mismatch.size, mismatch.hash, 0, 1)
			if body != nil || !errors.Is(err, errStreamObject) {
				t.Fatalf("wrong metadata accepted: %v", err)
			}
		}
		body, err := store.openStream(ctx, domain, "missing.bin", 1, hash, 0, 1)
		if body != nil || err == nil {
			t.Fatal("missing object was readable")
		}
	})
	t.Run("early_close_and_cancel", func(t *testing.T) {
		body, err := store.openStream(ctx, domain, "ranges.bin", int64(len(content)), hash, 0, int64(len(content)))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.ReadFull(body, make([]byte, 17)); err != nil {
			t.Fatal(err)
		}
		if err := body.Close(); err != nil {
			t.Fatal(err)
		}
		canceled, stop := context.WithCancel(ctx)
		stop()
		if body, err := store.openStream(canceled, domain, "ranges.bin", int64(len(content)), hash, 0, 1); body != nil || !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled read: %v", err)
		}
		if _, err := store.putStream(canceled, domain, "canceled.bin", strings.NewReader("x"), 1, "text/plain", streamDigest([]byte("x"))); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled write: %v", err)
		}
	})
	t.Run("etag_change_between_stat_and_get", func(t *testing.T) {
		const key = "etag-race.bin"
		old := []byte("synthetic original")
		if _, err := store.putStream(ctx, domain, key, bytes.NewReader(old), int64(len(old)), "application/octet-stream", streamDigest(old)); err != nil {
			t.Fatal(err)
		}
		replaced := false
		race := &minioStreamIntegrationTransport{base: transport}
		race.after = func(req *http.Request) error {
			if req.Method == http.MethodHead && strings.HasSuffix(req.URL.Path, "/"+key) && !replaced {
				replaced = true
				replacement := []byte("synthetic modified")
				_, err := store.putStream(ctx, domain, key, bytes.NewReader(replacement), int64(len(replacement)), "application/octet-stream", streamDigest(replacement))
				return err
			}
			return nil
		}
		pinned := &MinioStorage{client: newClient(race), bucket: bucket, prefixPath: "stage2r4"}
		body, err := pinned.openStream(ctx, domain, key, int64(len(old)), streamDigest(old), 0, int64(len(old)))
		if body != nil || !replaced || race.failure != nil || minio.ToErrorResponse(err).Code != "PreconditionFailed" {
			t.Fatalf("object changed after stat: replaced=%t code=%s hook_failed=%t", replaced, minio.ToErrorResponse(err).Code, race.failure != nil)
		}
	})
}

func sha256Pattern(t *testing.T, size int64) string {
	t.Helper()
	hash := sha256.New()
	if _, err := io.CopyBuffer(hash, &minioPatternReader{remaining: size}, make([]byte, 64<<10)); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(hash.Sum(nil))
}
