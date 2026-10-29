package parser

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
)

const (
	DocumentStructureChildFlag = "--document-structure-child"
	documentChildVersion       = "document-structure-child/1"
	documentChildInputLimit    = 8 << 20
	documentChildOutputLimit   = 16 << 20
	documentChildControlLimit  = 32 << 10
	documentChildStderrLimit   = 4 << 10
)

var (
	ErrDocumentChildUnresolved  = errors.New("document child exit or cleanup is unproven")
	ErrDocumentChildUnavailable = errors.New("document child execution is unavailable")
)

// DocumentChildControl is private local process input, never an HTTP DTO. Run
// supplies Version, Token and ParentPID; callers provide the fixed task inputs.
type DocumentChildControl struct {
	Version        string `json:"version"`
	TaskID         string `json:"task_id"`
	Attempt        int    `json:"attempt"`
	Format         string `json:"format"`
	Filename       string `json:"filename,omitempty"`
	ProfileID      string `json:"profile_id"`
	ProducerSHA256 string `json:"producer_sha256"`
	InputSHA256    string `json:"input_sha256"`
	InputBytes     int64  `json:"input_bytes"`
	Token          string `json:"token"`
	ParentPID      int    `json:"parent_pid"`
}

// DocumentChildOwnership is persisted privately by the shared task worker. It
// does not participate in public task payloads, producer or representation IDs.
type DocumentChildOwnership struct {
	Version          string `json:"version"`
	TaskID           string `json:"task_id"`
	Attempt          int    `json:"attempt"`
	Token            string `json:"token"`
	PID              int    `json:"pid"`
	PGID             int    `json:"pgid"`
	StartTicks       uint64 `json:"start_ticks"`
	BootID           string `json:"boot_id"`
	Directory        string `json:"directory"`
	InputSHA256      string `json:"input_sha256"`
	InputBytes       int64  `json:"input_bytes"`
	ParentPID        int    `json:"parent_pid"`
	ParentStartTicks uint64 `json:"parent_start_ticks"`
	ExecutableDevice uint64 `json:"executable_device"`
	ExecutableInode  uint64 `json:"executable_inode"`
}

// DocumentChildError contains only a frozen controlled failure code. Native
// diagnostics, paths, source bytes and environment values are never exposed.
type DocumentChildError struct {
	Code  string
	cause error
	// Private diagnostics contain only fixed stage labels, counts and process
	// exit status. Error/FailureCode and every wire schema stay unchanged.
	stage          string
	probe          string
	exitCode       int
	exitSignal     int
	callbackCalled bool
	stdoutBytes    int
	cleanupStage   string
}

func (e *DocumentChildError) Error() string       { return "document child: " + e.Code }
func (e *DocumentChildError) Unwrap() error       { return e.cause }
func (e *DocumentChildError) FailureCode() string { return e.Code }

type DocumentChildParser func(context.Context, DocumentChildControl, []byte) (json.RawMessage, error)

func IsDocumentStructureChild(args []string) bool {
	return len(args) == 1 && args[0] == DocumentStructureChildFlag
}

func documentChildHash(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			if char < 'a' || char > 'f' {
				return false
			}
		}
	}
	return true
}

func documentChildControlValid(control DocumentChildControl, complete bool) bool {
	profiles := map[string]string{"pdf": "pdf-plain-pages/1", "pptx": "pptx-ooxml-slides/1", "docx": "docx-ooxml-blocks/1"}
	profile, supported := profiles[control.Format]
	minimum := int64(1)
	maximum := int64(documentChildInputLimit)
	if control.ProfileID == KnowledgeParsedLocalProfile {
		profile, supported = KnowledgeParsedLocalProfile, KnowledgeParsedLocalFormat(control.Format) && knowledgeParsedFilename(control.Format, control.Filename)
		minimum, maximum = 0, 2<<20
	} else if control.Filename != "" {
		return false
	}
	if !supported || profile != control.ProfileID || !documentChildHash(control.TaskID, 32) || control.Attempt < 1 || control.Attempt > 3 || !documentChildHash(control.ProducerSHA256, 64) || !documentChildHash(control.InputSHA256, 64) || control.InputBytes < minimum || control.InputBytes > maximum {
		return false
	}
	if complete {
		return control.Version == documentChildVersion && documentChildHash(control.Token, 64) && control.ParentPID > 0
	}
	return (control.Version == "" || control.Version == documentChildVersion) && control.Token == "" && control.ParentPID == 0
}

func documentChildFailure(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "native_timeout"
	}
	var failure interface{ FailureCode() string }
	if errors.As(err, &failure) {
		switch failure.FailureCode() {
		case "original_unavailable", "original_integrity", "unsupported_document", "encrypted_document", "source_structure_unavailable", "invalid_source_structure", "budget_exceeded", "native_timeout", "native_process_failed", "source_changed", "lifecycle_changed", "policy_changed", "head_changed", "producer_changed", "document_integrity", "cleanup_failed":
			return failure.FailureCode()
		}
	}
	return "native_process_failed"
}

func documentChildError(code string, cause error) error {
	return &DocumentChildError{Code: code, cause: cause}
}

func documentChildDecode(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return documentChildError("native_process_failed", nil)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return documentChildError("native_process_failed", nil)
	}
	return nil
}

type documentChildReply struct {
	Version        string          `json:"version"`
	TaskID         string          `json:"task_id"`
	Attempt        int             `json:"attempt"`
	Token          string          `json:"token"`
	ProducerSHA256 string          `json:"producer_sha256"`
	InputSHA256    string          `json:"input_sha256"`
	InputBytes     int64           `json:"input_bytes"`
	Result         json.RawMessage `json:"result"`
	FailureCode    *string         `json:"failure_code"`
}

func documentChildReadReply(raw []byte, control DocumentChildControl) (json.RawMessage, error) {
	if len(raw) == 0 || len(raw) > documentChildOutputLimit {
		return nil, documentChildError("budget_exceeded", nil)
	}
	var reply documentChildReply
	if documentChildDecode(raw, &reply) != nil || reply.Version != documentChildVersion || reply.TaskID != control.TaskID || reply.Attempt != control.Attempt || reply.Token != control.Token || reply.ProducerSHA256 != control.ProducerSHA256 || reply.InputSHA256 != control.InputSHA256 || reply.InputBytes != control.InputBytes {
		return nil, documentChildError("native_process_failed", nil)
	}
	if reply.FailureCode != nil {
		if !bytes.Equal(bytes.TrimSpace(reply.Result), []byte("null")) || documentChildFailure(documentChildError(*reply.FailureCode, nil)) != *reply.FailureCode {
			return nil, documentChildError("native_process_failed", nil)
		}
		return nil, documentChildError(*reply.FailureCode, nil)
	}
	result := bytes.TrimSpace(reply.Result)
	if len(result) == 0 || result[0] != '{' || !json.Valid(result) {
		return nil, documentChildError("native_process_failed", nil)
	}
	return append(json.RawMessage(nil), result...), nil
}

// documentChildBuffer either rejects overflow (stdout) or drains while retaining
// only the first bounded bytes (stderr). It never blocks a process pipe writer.
type documentChildBuffer struct {
	mu       sync.Mutex
	data     []byte
	limit    int
	truncate bool
	exceeded bool
	overflow chan struct{}
}

func newDocumentChildBuffer(limit int, truncate bool) *documentChildBuffer {
	return &documentChildBuffer{limit: limit, truncate: truncate, overflow: make(chan struct{})}
}

func (buffer *documentChildBuffer) Write(raw []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	count := len(raw)
	remaining := buffer.limit - len(buffer.data)
	if count > remaining {
		buffer.data = append(buffer.data, raw[:remaining]...)
		if !buffer.exceeded {
			buffer.exceeded = true
			close(buffer.overflow)
		}
		if !buffer.truncate {
			return remaining, io.ErrShortWrite
		}
	} else {
		buffer.data = append(buffer.data, raw...)
	}
	return count, nil
}

func (buffer *documentChildBuffer) bytes() []byte {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return append([]byte(nil), buffer.data...)
}

func (buffer *documentChildBuffer) isExceeded() bool {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.exceeded
}

func (buffer *documentChildBuffer) length() int {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return len(buffer.data)
}
