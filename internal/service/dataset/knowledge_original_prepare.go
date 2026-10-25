// Copyright 2026 LFen's Second Brain contributors.
// SPDX-License-Identifier: Apache-2.0

package dataset

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"os"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	"ragflow/internal/common"
	"ragflow/internal/utility"

	"github.com/zeebo/xxh3"
)

var (
	errOriginalOptions = errors.New("invalid original preparation options")
	errOriginalType    = errors.New("unsupported original format")
	errOriginalLength  = errors.New("original length mismatch")
	errOriginalDigest  = errors.New("original digest mismatch")
	errOriginalText    = errors.New("invalid original text")
	errOriginalSecret  = errors.New("original contains a restricted marker")
)

// Limits are supplied by an owning admission policy, not an HTTP configuration.
type knowledgeOriginalLimits struct {
	fileBytes            int64
	officeEntries        int
	officeExpandedBytes  uint64
	officeRatio          uint64
	officeXMLBytes       int64
	officeAllXMLBytes    int64
	officeDirectoryBytes int64
	officeXMLDepth       int
}

func (l knowledgeOriginalLimits) isValid() bool {
	return l.fileBytes > 0 && l.fileBytes < math.MaxInt64 && l.officeEntries > 0 && l.officeEntries < 65535 &&
		l.officeExpandedBytes > 0 && l.officeRatio > 0 && l.officeXMLBytes > 0 && l.officeXMLBytes < math.MaxInt64 &&
		l.officeAllXMLBytes >= l.officeXMLBytes && l.officeDirectoryBytes > 0 && l.officeXMLDepth > 0
}

type knowledgeOriginalStage struct {
	file        *os.File
	root        *os.Root
	directory   string
	bodyCreated bool
	byteLength  int64
	sha256      string
	nativeHash  string
	mediaType   string
}

// close removes only this stage's generated file and directory through its
// pinned root. It does not delete source data or stored version objects.
func (s *knowledgeOriginalStage) close() error {
	if s.root == nil {
		return nil
	}
	var errs []error
	if s.file != nil {
		errs = append(errs, s.file.Close())
		s.file = nil
	}
	if s.directory != "" {
		names := []string{s.directory}
		if s.bodyCreated {
			names = []string{s.directory + "/body", s.directory}
		}
		for _, name := range names {
			if err := s.root.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
				errs = append(errs, err)
			}
		}
	}
	errs = append(errs, s.root.Close())
	s.root = nil
	return errors.Join(errs...)
}

func classifyKnowledgeOriginalMedia(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".pdf":
		return "application/pdf"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".pptx":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	default:
		if utility.IsKnowledgeTextSource(name, nil) {
			return "text/plain"
		}
		return ""
	}
}

func inspectKnowledgeOriginalNames(filename, relativePath, declaredType string) (string, error) {
	if filename == "" || !utf8.ValidString(filename) || utf8.RuneCountInString(filename) > 256 || filename == "." || filename == ".." ||
		strings.ContainsAny(filename, "/\\:") || strings.ContainsFunc(filename, unicode.IsControl) ||
		!isKnowledgeImportPath(filename) || !isKnowledgeImportPath(relativePath) || strings.ContainsAny(relativePath, "*?[]{}") || strings.ContainsFunc(relativePath, unicode.IsControl) {
		return "", errOriginalType
	}
	mediaType := classifyKnowledgeOriginalMedia(filename)
	if mediaType == "" || classifyKnowledgeOriginalMedia(relativePath) != mediaType || (declaredType != mediaType && declaredType != "application/octet-stream") {
		return "", errOriginalType
	}
	return mediaType, nil
}

// stageKnowledgeOriginal preserves measured bytes in an owner-only temporary
// file. It never registers a source, publishes a revision, or starts ingestion.
func stageKnowledgeOriginal(ctx context.Context, scratchDir, filename, relativePath, declaredType string, source io.Reader, expectedBytes int64, expectedSHA256 string, limits knowledgeOriginalLimits) (stage *knowledgeOriginalStage, err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !limits.isValid() || source == nil || expectedBytes < 1 || expectedBytes > limits.fileBytes || (expectedSHA256 != "" && !isKnowledgeID(expectedSHA256, 64)) {
		return nil, errOriginalOptions
	}
	mediaType, err := inspectKnowledgeOriginalNames(filename, relativePath, declaredType)
	if err != nil {
		return nil, err
	}
	root, _, err := utility.OpenKnowledgeDirectoryRoot(scratchDir)
	if err != nil {
		return nil, err
	}
	prepared := &knowledgeOriginalStage{root: root, mediaType: mediaType}
	defer func() {
		if err != nil {
			err = errors.Join(err, prepared.close())
		}
	}()
	prepared.directory = "original-" + common.GenerateUUID()
	if err = root.Mkdir(prepared.directory, 0700); err != nil {
		prepared.directory = ""
		return nil, err
	}
	prepared.file, err = root.OpenFile(prepared.directory+"/body", os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	prepared.bodyCreated = true
	digest, native := sha256.New(), xxh3.New()
	probe := &knowledgeSourceProbe{valid: true}
	var target io.Writer = io.MultiWriter(prepared.file, digest, native)
	if mediaType == "text/plain" {
		target = io.MultiWriter(prepared.file, digest, native, probe)
	}
	prepared.byteLength, err = copyKnowledgeOriginalBytes(ctx, target, source, expectedBytes)
	if err != nil {
		return nil, err
	}
	prepared.sha256 = hex.EncodeToString(digest.Sum(nil))
	nativeSum := native.Sum128().Bytes()
	prepared.nativeHash = hex.EncodeToString(nativeSum[:])
	if expectedSHA256 != "" && prepared.sha256 != expectedSHA256 {
		return nil, errOriginalDigest
	}
	if mediaType == "text/plain" {
		if !probe.valid || probe.nul || len(probe.pending) != 0 {
			return nil, errOriginalText
		}
		if probe.secret {
			return nil, errOriginalSecret
		}
	} else if mediaType == "application/pdf" {
		if err = inspectKnowledgeOriginalPDF(prepared.file, expectedBytes); err != nil {
			return nil, err
		}
	} else if err = inspectKnowledgeOriginalOffice(ctx, prepared.file, expectedBytes, mediaType, limits); err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if err = prepared.file.Sync(); err != nil {
		return nil, err
	}
	if _, err = prepared.file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	return prepared, nil
}

func copyKnowledgeOriginalBytes(ctx context.Context, target io.Writer, source io.Reader, expectedBytes int64) (int64, error) {
	reader := &knowledgeContextReader{ctx: ctx, reader: source}
	n, err := io.CopyBuffer(target, io.LimitReader(reader, expectedBytes), make([]byte, 64<<10))
	if err != nil {
		return n, err
	}
	if n != expectedBytes {
		return n, errOriginalLength
	}
	// Detect excess bytes without writing past the reserved file length.
	var extra [1]byte
	if count, err := io.ReadFull(reader, extra[:]); count != 0 {
		return n, errOriginalLength
	} else if err != io.EOF {
		return n, err
	}
	return n, nil
}

func inspectKnowledgeOriginalPDF(source io.ReaderAt, size int64) error {
	if size < 8 {
		return errOriginalType
	}
	var header [8]byte
	if _, err := source.ReadAt(header[:], 0); err != nil {
		return err
	}
	if !((string(header[:7]) == "%PDF-1." && header[7] >= '0' && header[7] <= '7') || string(header[:]) == "%PDF-2.0") {
		return errOriginalType
	}
	tail := make([]byte, min(size, 1024))
	if _, err := source.ReadAt(tail, size-int64(len(tail))); err != nil {
		return err
	}
	if !bytes.HasSuffix(bytes.TrimSpace(tail), []byte("%%EOF")) {
		return errOriginalType
	}
	return nil
}
