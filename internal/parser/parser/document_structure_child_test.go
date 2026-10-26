package parser

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

func documentChildTestControl() DocumentChildControl {
	return DocumentChildControl{TaskID: strings.Repeat("1", 32), Attempt: 1, Format: "pdf", ProfileID: "pdf-plain-pages/1", ProducerSHA256: strings.Repeat("2", 64), InputSHA256: strings.Repeat("3", 64), InputBytes: 7}
}

func TestDocumentChildControlBudgetAndBindings(t *testing.T) {
	base := documentChildTestControl()
	if !documentChildControlValid(base, false) {
		t.Fatal("valid fixed control rejected")
	}
	tests := map[string]func(*DocumentChildControl){
		"unknown_format":  func(value *DocumentChildControl) { value.Format = "html" },
		"wrong_profile":   func(value *DocumentChildControl) { value.ProfileID = "pptx-ooxml-slides/1" },
		"attempt_zero":    func(value *DocumentChildControl) { value.Attempt = 0 },
		"attempt_four":    func(value *DocumentChildControl) { value.Attempt = 4 },
		"empty_input":     func(value *DocumentChildControl) { value.InputBytes = 0 },
		"large_input":     func(value *DocumentChildControl) { value.InputBytes = documentChildInputLimit + 1 },
		"bad_sha":         func(value *DocumentChildControl) { value.InputSHA256 = strings.Repeat("Z", 64) },
		"injected_token":  func(value *DocumentChildControl) { value.Token = strings.Repeat("a", 64) },
		"injected_parent": func(value *DocumentChildControl) { value.ParentPID = 50 },
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			value := base
			change(&value)
			if documentChildControlValid(value, false) {
				t.Fatal("invalid control accepted")
			}
		})
	}
	base.Version, base.Token, base.ParentPID = documentChildVersion, strings.Repeat("a", 64), 20
	if !documentChildControlValid(base, true) || documentChildControlValid(base, false) {
		t.Fatal("complete child control and caller control are not separate")
	}
	if !IsDocumentStructureChild([]string{DocumentStructureChildFlag}) || IsDocumentStructureChild([]string{DocumentStructureChildFlag, "--api"}) || IsDocumentStructureChild(nil) {
		t.Fatal("private mode dispatch must match one exact flag")
	}
}

func TestDocumentChildBoundedPipeBuffers(t *testing.T) {
	stdout := newDocumentChildBuffer(4, false)
	if count, err := stdout.Write([]byte("abc")); count != 3 || err != nil {
		t.Fatal("in-budget stdout was rejected")
	}
	if count, err := stdout.Write([]byte("def")); count != 1 || !errors.Is(err, io.ErrShortWrite) || !stdout.isExceeded() || !bytes.Equal(stdout.bytes(), []byte("abcd")) {
		t.Fatal("stdout overflow did not retain the bounded prefix and fail")
	}
	select {
	case <-stdout.overflow:
	default:
		t.Fatal("overflow did not wake the process owner")
	}
	stderr := newDocumentChildBuffer(4, true)
	if count, err := stderr.Write([]byte("private diagnostics")); count != 19 || err != nil || len(stderr.bytes()) != 4 || !stderr.isExceeded() {
		t.Fatal("stderr must drain while retaining at most its configured bytes")
	}
	if count, err := stderr.Write([]byte("more")); count != 4 || err != nil || len(stderr.bytes()) != 4 {
		t.Fatal("continued stderr should not block or grow the diagnostic budget")
	}
}

func TestDocumentChildReplyRequiresOriginalIdentity(t *testing.T) {
	control := documentChildTestControl()
	control.Version, control.Token, control.ParentPID = documentChildVersion, strings.Repeat("a", 64), 20
	reply := documentChildReply{Version: control.Version, TaskID: control.TaskID, Attempt: control.Attempt, Token: control.Token, ProducerSHA256: control.ProducerSHA256, InputSHA256: control.InputSHA256, InputBytes: control.InputBytes, Result: json.RawMessage(`{"text":"a—b"}`)}
	encoded, err := json.Marshal(reply)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := documentChildReadReply(encoded, control); err != nil || !bytes.Equal(result, reply.Result) {
		t.Fatal("valid original bound reply rejected")
	}
	for _, name := range []string{"task", "attempt", "token", "producer", "input_sha", "input_bytes"} {
		t.Run(name, func(t *testing.T) {
			changed := reply
			switch name {
			case "task":
				changed.TaskID = strings.Repeat("b", 32)
			case "attempt":
				changed.Attempt++
			case "token":
				changed.Token = strings.Repeat("b", 64)
			case "producer":
				changed.ProducerSHA256 = strings.Repeat("b", 64)
			case "input_sha":
				changed.InputSHA256 = strings.Repeat("b", 64)
			case "input_bytes":
				changed.InputBytes++
			}
			raw, _ := json.Marshal(changed)
			if _, err := documentChildReadReply(raw, control); err == nil {
				t.Fatal("mismatched child identity was accepted")
			}
		})
	}
	for _, raw := range [][]byte{append(encoded, []byte(`{}`)...), []byte(`{"unknown":true}`), []byte(`null`)} {
		if _, err := documentChildReadReply(raw, control); err == nil {
			t.Fatal("malformed/extra protocol output accepted")
		}
	}
	code := "budget_exceeded"
	reply.FailureCode, reply.Result = &code, json.RawMessage("null")
	raw, _ := json.Marshal(reply)
	_, err = documentChildReadReply(raw, control)
	var failure *DocumentChildError
	if !errors.As(err, &failure) || failure.Code != code {
		t.Fatal("controlled parser failure was not retained")
	}
	code = "raw-native-error-with-private-path"
	raw, _ = json.Marshal(reply)
	if _, err := documentChildReadReply(raw, control); err == nil || strings.Contains(err.Error(), "private-path") {
		t.Fatal("uncontrolled diagnostic was exposed")
	}
}

func TestDocumentChildFailureDoesNotExposeRawDiagnostics(t *testing.T) {
	if code := documentChildFailure(errors.New("raw source secret")); code != "native_process_failed" {
		t.Fatal("raw error escaped controlled mapping")
	}
	err := documentChildError("native_timeout", context.DeadlineExceeded)
	if !errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "deadline") || documentChildFailure(err) != "native_timeout" {
		t.Fatal("timeout cause/code was not preserved independently of raw text")
	}
	if !errors.Is(documentChildError("cleanup_failed", ErrDocumentChildUnresolved), ErrDocumentChildUnresolved) {
		t.Fatal("unresolved cleanup must preserve the shared execution fence")
	}
}
