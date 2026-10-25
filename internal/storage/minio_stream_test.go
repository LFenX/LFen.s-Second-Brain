// Copyright 2026 LFen's Second Brain contributors.
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"runtime"
	"strings"
	"testing"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type minioStreamTransport func(*http.Request) (*http.Response, error)

func (f minioStreamTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func newMinioStreamStorage(t *testing.T, transport minioStreamTransport) *MinioStorage {
	t.Helper()
	client, err := minio.New("objects.invalid", &minio.Options{
		Region:       "us-east-1",
		Secure:       true,
		Creds:        credentials.NewStaticV4("synthetic-access", "synthetic-secret", ""),
		Transport:    transport,
		BucketLookup: minio.BucketLookupPath,
		MaxRetries:   1,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &MinioStorage{client: client, bucket: "fixture-bucket", prefixPath: "fixtures"}
}

func minioStreamResponse(r *http.Request, status int, header http.Header, body io.ReadCloser) *http.Response {
	if header == nil {
		header = make(http.Header)
	}
	if body == nil {
		body = io.NopCloser(strings.NewReader(""))
	}
	return &http.Response{StatusCode: status, Status: fmt.Sprintf("%d %s", status, http.StatusText(status)), Header: header, Body: body, Request: r}
}

func minioStreamHeaders(size int64, digest, etag string) http.Header {
	header := make(http.Header)
	header.Set("Content-Length", fmt.Sprint(size))
	header.Set("Content-Type", "application/pdf")
	header.Set("Last-Modified", "Mon, 26 Oct 2026 00:00:00 GMT")
	header.Set("ETag", `"`+etag+`"`)
	header.Set("X-Amz-Meta-Sha256", digest)
	return header
}

func streamDigest(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}

type minioPatternReader struct {
	remaining int64
	readBytes int64
	maxBuffer int
}

func (r *minioPatternReader) Read(p []byte) (int, error) {
	if len(p) > r.maxBuffer {
		r.maxBuffer = len(p)
	}
	if len(p) > 1<<20 {
		return 0, errors.New("SDK requested a whole-object-sized input buffer")
	}
	if r.remaining == 0 {
		return 0, io.EOF
	}
	n := min(int64(len(p)), r.remaining)
	for i := range p[:n] {
		p[i] = 0xa7
	}
	r.remaining -= n
	r.readBytes += n
	return int(n), nil
}

func TestMinioStreamPutBounded(t *testing.T) {
	for _, size := range []int64{0, 3 << 20, 64 << 20} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			hash := sha256.New()
			if _, err := io.CopyBuffer(hash, &minioPatternReader{remaining: size}, make([]byte, 64<<10)); err != nil {
				t.Fatal(err)
			}
			digest := hex.EncodeToString(hash.Sum(nil))
			calls := 0
			store := newMinioStreamStorage(t, func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != http.MethodPut || r.URL.Path != "/fixture-bucket/fixtures/domain/source.bin" || r.URL.RawQuery != "" {
					t.Fatalf("unexpected object request: %s %s", r.Method, r.URL.Path)
				}
				if r.ContentLength != size || r.Header.Get("X-Amz-Meta-Sha256") != digest {
					t.Fatal("length or digest metadata lost")
				}
				if r.Header.Get("Content-Md5") != "" {
					t.Fatal("whole-object MD5 buffering enabled")
				}
				if r.Header.Get("If-None-Match") != "*" {
					t.Fatal("immutable write condition missing")
				}
				actual := sha256.New()
				var count int64
				var err error
				if r.Body != nil {
					count, err = io.CopyBuffer(actual, r.Body, make([]byte, 64<<10))
				}
				if err != nil || count != size || hex.EncodeToString(actual.Sum(nil)) != digest {
					t.Fatalf("stream bytes changed: count=%d err=%v", count, err)
				}
				return minioStreamResponse(r, http.StatusOK, minioStreamHeaders(0, digest, "stored"), nil), nil
			})
			source := &minioPatternReader{remaining: size}
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			info, err := store.PutImmutableStream(t.Context(), "domain", "source.bin", source, size, "application/octet-stream", digest)
			runtime.ReadMemStats(&after)
			if err != nil || info.Size != size || source.readBytes != size || calls != 1 {
				t.Fatalf("put failed: size=%d read=%d calls=%d err=%v", info.Size, source.readBytes, calls, err)
			}
			if source.maxBuffer > 1<<20 {
				t.Fatalf("input buffer grew to %d", source.maxBuffer)
			}
			allocated := after.TotalAlloc - before.TotalAlloc
			if allocated > 4<<20 {
				t.Fatalf("one stream allocated %d bytes for a %d-byte source", allocated, size)
			}
			t.Logf("source_bytes=%d largest_source_read=%d allocation_bytes=%d", size, source.maxBuffer, allocated)
		})
	}
}

func TestMinioStreamPutCreatesBucket(t *testing.T) {
	var methods []string
	store := newMinioStreamStorage(t, func(r *http.Request) (*http.Response, error) {
		methods = append(methods, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodHead {
			return minioStreamResponse(r, http.StatusNotFound, nil, nil), nil
		}
		if r.Body != nil {
			if _, err := io.Copy(io.Discard, r.Body); err != nil {
				return nil, err
			}
		}
		return minioStreamResponse(r, http.StatusOK, minioStreamHeaders(0, streamDigest([]byte("x")), "stored"), nil), nil
	})
	store.bucket, store.prefixPath = "", ""
	if _, err := store.PutImmutableStream(t.Context(), "new-bucket", "sample.bin", strings.NewReader("x"), 1, "text/plain", streamDigest([]byte("x"))); err != nil {
		t.Fatal(err)
	}
	want := []string{"HEAD /new-bucket/", "PUT /new-bucket/", "PUT /new-bucket/sample.bin"}
	if fmt.Sprint(methods) != fmt.Sprint(want) {
		t.Fatalf("bucket creation sequence: %v", methods)
	}
}

func TestMinioStreamPutLimitsAndFailures(t *testing.T) {
	t.Run("extra bytes remain with caller", func(t *testing.T) {
		source := strings.NewReader("original-extra")
		store := newMinioStreamStorage(t, func(r *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(r.Body)
			if err != nil || string(body) != "original" {
				t.Fatalf("expected declared prefix: %q %v", body, err)
			}
			return minioStreamResponse(r, http.StatusOK, minioStreamHeaders(0, "", "stored"), nil), nil
		})
		if _, err := store.PutImmutableStream(t.Context(), "domain", "item", source, 8, "text/plain", streamDigest([]byte("original"))); err != nil {
			t.Fatal(err)
		}
		if source.Len() != len("-extra") {
			t.Fatal("transport consumed bytes beyond the measured source")
		}
	})
	t.Run("short reader", func(t *testing.T) {
		store := newMinioStreamStorage(t, func(r *http.Request) (*http.Response, error) {
			if _, err := io.Copy(io.Discard, r.Body); err != nil {
				return nil, err
			}
			return minioStreamResponse(r, http.StatusOK, minioStreamHeaders(0, "", "stored"), nil), nil
		})
		_, err := store.PutImmutableStream(t.Context(), "domain", "item", strings.NewReader("short"), 100, "text/plain", streamDigest([]byte("short")))
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("short body reported success: %v", err)
		}
	})
	t.Run("transport failure", func(t *testing.T) {
		failure := errors.New("synthetic transport failure")
		calls := 0
		store := newMinioStreamStorage(t, func(r *http.Request) (*http.Response, error) { calls++; return nil, failure })
		if _, err := store.PutImmutableStream(t.Context(), "domain", "item", strings.NewReader("x"), 1, "text/plain", streamDigest([]byte("x"))); err == nil || calls != 1 {
			t.Fatalf("failure/retry mismatch: calls=%d err=%v", calls, err)
		}
	})
	for _, test := range []struct {
		name string
		size int64
		hash string
	}{
		{"negative size", -1, strings.Repeat("a", 64)},
		{"missing digest", 1, ""},
		{"noncanonical digest", 1, strings.Repeat("A", 64)},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := newMinioStreamStorage(t, func(*http.Request) (*http.Response, error) { t.Fatal("invalid metadata sent HTTP"); return nil, nil })
			if _, err := store.PutImmutableStream(t.Context(), "domain", "item", strings.NewReader("x"), test.size, "text/plain", test.hash); !errors.Is(err, errStreamMetadata) {
				t.Fatal(err)
			}
		})
	}
}

func TestMinioStreamReadExactBinaryRanges(t *testing.T) {
	content := []byte{0xef, 0xbb, 0xbf, 0xf0, 0x9f, 0x98, 0xba, 0x0d, 0x0a, 0xff}
	digest := streamDigest(content)
	for _, bounds := range [][2]int64{{0, 10}, {0, 1}, {4, 2}, {9, 1}, {10, 0}} {
		t.Run(fmt.Sprint(bounds), func(t *testing.T) {
			getCalls := 0
			store := newMinioStreamStorage(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.Path != "/fixture-bucket/fixtures/domain/item" {
					t.Fatal("prefix mapping lost")
				}
				if r.Method == http.MethodHead {
					return minioStreamResponse(r, http.StatusOK, minioStreamHeaders(10, digest, "fixed-etag"), nil), nil
				}
				getCalls++
				start, length := bounds[0], bounds[1]
				end := start + length - 1
				if r.Method != http.MethodGet || r.Header.Get("If-Match") != `"fixed-etag"` || r.Header.Get("Range") != fmt.Sprintf("bytes=%d-%d", start, end) {
					t.Fatal("fixed object or exact range not pinned")
				}
				header := minioStreamHeaders(length, digest, "fixed-etag")
				header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/10", start, end))
				return minioStreamResponse(r, http.StatusPartialContent, header, io.NopCloser(bytes.NewReader(content[start:start+length]))), nil
			})
			body, err := store.OpenStream(t.Context(), "domain", "item", 10, digest, bounds[0], bounds[1])
			if err != nil {
				t.Fatal(err)
			}
			defer body.Close()
			got, err := io.ReadAll(body)
			if err != nil || !bytes.Equal(got, content[bounds[0]:bounds[0]+bounds[1]]) {
				t.Fatalf("binary range changed: %v %v", got, err)
			}
			if bounds[1] == 0 && getCalls != 0 {
				t.Fatal("zero-length read performed GET")
			}
		})
	}
}

type minioTrackedBody struct {
	io.Reader
	closed int
}

func (b *minioTrackedBody) Close() error { b.closed++; return nil }

func TestMinioStreamReadRejectsChangedObject(t *testing.T) {
	digest := streamDigest([]byte("12345678"))
	for _, scenario := range []string{"wrong size", "wrong digest", "missing etag", "changed etag", "ignored range", "wrong range", "truncated body"} {
		t.Run(scenario, func(t *testing.T) {
			getCalls := 0
			tracked := &minioTrackedBody{Reader: strings.NewReader("3456")}
			store := newMinioStreamStorage(t, func(r *http.Request) (*http.Response, error) {
				if r.Method == http.MethodHead {
					header := minioStreamHeaders(8, digest, "fixed")
					switch scenario {
					case "wrong size":
						header.Set("Content-Length", "9")
					case "wrong digest":
						header.Set("X-Amz-Meta-Sha256", strings.Repeat("0", 64))
					case "missing etag":
						header.Del("ETag")
					}
					return minioStreamResponse(r, http.StatusOK, header, nil), nil
				}
				getCalls++
				if scenario == "changed etag" {
					return minioStreamResponse(r, http.StatusPreconditionFailed, nil, io.NopCloser(strings.NewReader(`<Error><Code>PreconditionFailed</Code><Message>changed</Message></Error>`))), nil
				}
				header := minioStreamHeaders(4, digest, "fixed")
				header.Set("Content-Range", "bytes 2-5/8")
				if scenario == "ignored range" {
					header.Del("Content-Range")
				}
				if scenario == "wrong range" {
					header.Set("Content-Range", "bytes 1-4/8")
				}
				if scenario == "truncated body" {
					tracked.Reader = strings.NewReader("34")
				}
				return minioStreamResponse(r, http.StatusPartialContent, header, tracked), nil
			})
			body, err := store.OpenStream(t.Context(), "domain", "item", 8, digest, 2, 4)
			if scenario == "truncated body" {
				if err != nil {
					t.Fatal(err)
				}
				_, err = io.ReadAll(body)
				_ = body.Close()
				if !errors.Is(err, io.ErrUnexpectedEOF) {
					t.Fatalf("truncation masked: %v", err)
				}
			} else if err == nil || body != nil {
				t.Fatal("changed object accepted")
			}
			if (scenario == "ignored range" || scenario == "wrong range" || scenario == "truncated body") && tracked.closed != 1 {
				t.Fatalf("response body leaked: %d", tracked.closed)
			}
			if (scenario == "wrong size" || scenario == "wrong digest" || scenario == "missing etag") && getCalls != 0 {
				t.Fatal("invalid object issued GET")
			}
		})
	}
}

func TestMinioStreamRejectsRangeBeforeHTTP(t *testing.T) {
	for _, bounds := range [][2]int64{{-1, 1}, {0, -1}, {8, 1}, {math.MaxInt64, 1}, {1, math.MaxInt64}} {
		store := newMinioStreamStorage(t, func(*http.Request) (*http.Response, error) { t.Fatal("invalid range sent HTTP"); return nil, nil })
		if body, err := store.OpenStream(t.Context(), "domain", "item", 8, strings.Repeat("a", 64), bounds[0], bounds[1]); !errors.Is(err, errStreamRange) || body != nil {
			t.Fatalf("invalid range accepted: %v %v", bounds, err)
		}
	}
}

func TestMinioStreamCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	store := newMinioStreamStorage(t, func(*http.Request) (*http.Response, error) { t.Fatal("cancelled request sent HTTP"); return nil, nil })
	if _, err := store.PutImmutableStream(ctx, "domain", "item", strings.NewReader("x"), 1, "text/plain", streamDigest([]byte("x"))); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := store.OpenStream(ctx, "domain", "item", 1, streamDigest([]byte("x")), 0, 1); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	body := &minioStreamReader{ctx: ctx, body: io.NopCloser(strings.NewReader("x")), remaining: 1}
	if _, err := body.Read(make([]byte, 1)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	_ = body.Close()
}
