// Copyright 2026 LFen's Second Brain contributors.
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
)

var (
	errStreamMetadata = errors.New("invalid stream metadata")
	errStreamRange    = errors.New("invalid stream range")
	errStreamObject   = errors.New("stream object integrity mismatch")
)

func isStreamSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, c := range []byte(value) {
		if !('0' <= c && c <= '9') && !('a' <= c && c <= 'f') {
			return false
		}
	}
	return true
}

// putStream transports a previously measured and hashed source. It does not
// authorize a source, verify the supplied digest, or publish a catalogue row.
// A caller must use a private staging key until its admission checks succeed.
func (m *MinioStorage) putStream(ctx context.Context, bucket, key string, source io.Reader, size int64, contentType, sha256 string) (minio.UploadInfo, error) {
	if err := ctx.Err(); err != nil {
		return minio.UploadInfo{}, err
	}
	if m == nil || m.client == nil || source == nil || size < 0 || !isStreamSHA256(sha256) {
		return minio.UploadInfo{}, errStreamMetadata
	}
	bucket, key = m.resolveBucketAndPath(bucket, key)
	if m.bucket == "" {
		exists, err := m.client.BucketExists(ctx, bucket)
		if err != nil {
			return minio.UploadInfo{}, err
		}
		if !exists {
			if err := m.client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
				return minio.UploadInfo{}, err
			}
		}
	}
	// A known length and one PUT avoid multipart-sized buffers. Content MD5
	// must remain disabled: the SDK buffers non-seekable sources to compute it.
	limited := &io.LimitedReader{R: source, N: size}
	info, err := m.client.PutObject(ctx, bucket, key, limited, size, minio.PutObjectOptions{
		ContentType:      contentType,
		UserMetadata:     map[string]string{"sha256": sha256},
		DisableMultipart: true,
		SendContentMd5:   false,
	})
	if err == nil && limited.N != 0 {
		return minio.UploadInfo{}, io.ErrUnexpectedEOF
	}
	return info, err
}

// openStream pins a stored object before returning an exact byte range. Digest
// metadata is checked here; this does not recompute a whole-object digest from
// a partial read. Authorization and public HTTP Range handling belong to callers.
func (m *MinioStorage) openStream(ctx context.Context, bucket, key string, size int64, sha256 string, offset, length int64) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if m == nil || m.client == nil || size < 0 || !isStreamSHA256(sha256) {
		return nil, errStreamMetadata
	}
	if offset < 0 || length < 0 || offset > size || length > size-offset {
		return nil, errStreamRange
	}
	bucket, key = m.resolveBucketAndPath(bucket, key)
	info, err := m.client.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return nil, err
	}
	if info.Size != size || info.Metadata.Get("X-Amz-Meta-Sha256") != sha256 || info.ETag == "" {
		return nil, errStreamObject
	}
	if length == 0 {
		return io.NopCloser(bytes.NewReader(nil)), nil
	}
	opts := minio.GetObjectOptions{VersionID: info.VersionID}
	if err := opts.SetMatchETag(info.ETag); err != nil {
		return nil, err
	}
	end := offset + length - 1
	if err := opts.SetRange(offset, end); err != nil {
		return nil, err
	}
	// Core.GetObject starts the GET immediately. The ordinary Object reader is
	// lazy and would postpone status/range failures until after a caller's headers.
	body, actual, header, err := (minio.Core{Client: m.client}).GetObject(ctx, bucket, key, opts)
	if err != nil {
		return nil, err
	}
	wantRange := fmt.Sprintf("bytes %d-%d/%d", offset, end, size)
	if actual.Size != length || actual.ETag != info.ETag || header.Get("Content-Range") != wantRange ||
		header.Get("X-Amz-Meta-Sha256") != sha256 {
		_ = body.Close()
		return nil, errStreamObject
	}
	return &minioStreamReader{ctx: ctx, body: body, remaining: length}, nil
}

type minioStreamReader struct {
	ctx       context.Context
	body      io.ReadCloser
	remaining int64
}

func (r *minioStreamReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if r.remaining == 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.body.Read(p)
	r.remaining -= int64(n)
	if errors.Is(err, io.EOF) && r.remaining > 0 {
		err = io.ErrUnexpectedEOF
	}
	return n, err
}

func (r *minioStreamReader) Close() error { return r.body.Close() }

var _ io.ReadCloser = (*minioStreamReader)(nil)
