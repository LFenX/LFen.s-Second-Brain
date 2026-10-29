package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/service/dataset"
)

func TestKnowledgeParsedDiscoveryRequiresCompleteSource(t *testing.T) {
	valid := `{"contract_version":"0.2.0-candidate","query":{"source_ref":{"domain_id":"` + strings.Repeat("a", 32) + `","source_id":"` + strings.Repeat("b", 32) + `","source_revision_id":"` + strings.Repeat("c", 32) + `","source_sha256":"` + strings.Repeat("d", 64) + `"},"cursor":"","limit":20}}`
	for _, tc := range []struct {
		name, body string
		accepted   bool
	}{
		{"valid", valid, true},
		{"missing_source_sha", strings.Replace(valid, `,"source_sha256":"`+strings.Repeat("d", 64)+`"`, "", 1), false},
		{"unknown_scope", strings.Replace(valid, `"limit":20`, `"limit":20,"owner_id":"forged"`, 1), false},
		{"duplicate", strings.Replace(valid, `"limit":20`, `"limit":20,"limit":1`, 1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := contentTestContext(tc.body)
			_, ok := decodeContent[contentParsedRepresentationsRequest](c)
			if ok != tc.accepted {
				t.Fatal(ok)
			}
		})
	}
}

func TestKnowledgeParsedAdmissionDecodesOnlyFixedInputs(t *testing.T) {
	source := entity.KnowledgeContentSourceRef{DomainID: strings.Repeat("a", 32), SourceID: strings.Repeat("b", 32), RevisionID: strings.Repeat("c", 32), SHA256: strings.Repeat("d", 64)}
	task := contentParsedTaskRequest{Version: entity.KnowledgeContentVersion, Request: entity.KnowledgeParsedTaskRequest{Source: source, ExpectedLifecycleVersion: 1, ExpectedPolicyVersion: 1, ExpectedContentPolicyVersion: 2, ExpectedProducerSHA256: strings.Repeat("e", 64), MaxUnitBytes: 128, Confirmed: true, IdempotencyKey: "synthetic-task"}}
	raw, err := json.Marshal(task)
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{string(raw), strings.Replace(string(raw), `"max_unit_bytes":128`, `"max_unit_bytes":128,"ParseResult":{}`, 1), strings.Replace(string(raw), `"max_unit_bytes":128`, `"max_unit_bytes":128,"File":{}`, 1), strings.Replace(string(raw), `"max_unit_bytes":128`, `"max_unit_bytes":128,"Extra":{"owner_id":"forged"}`, 1), strings.Replace(string(raw), `"max_unit_bytes":128`, `"max_unit_bytes":128,"max_unit_bytes":8`, 1), strings.Replace(string(raw), `"max_unit_bytes":128`, `"max_unit_bytes":1.5`, 1)} {
		c, _ := contentTestContext(body)
		_, ok := decodeContent[contentParsedTaskRequest](c)
		if ok != (body == string(raw)) {
			t.Fatal("untrusted producer facts crossed task admission", ok)
		}
	}
	for _, value := range []any{
		contentPolicyRequest{Version: entity.KnowledgeContentVersion, Request: entity.KnowledgeContentPolicyRequest{ExpectedPolicyVersion: 1, ExpectedContentPolicyVersion: 1, ParseEnabled: true, Confirmed: true, IdempotencyKey: "enable"}},
		contentParsedSnapshotRequest{Version: entity.KnowledgeContentVersion, Request: entity.KnowledgeParsedSnapshotRequest{DocumentID: source.SourceID, ExpectedPolicyVersion: 1, RelativePath: "rows.csv", Classification: entity.KnowledgeDirectoryClassification{ComponentSide: "non_code"}}},
		contentParsedSourceRequest{Version: entity.KnowledgeContentVersion, SourceID: source.SourceID},
		contentParsedTaskGetRequest{Version: entity.KnowledgeContentVersion, TaskID: strings.Repeat("f", 32)},
		contentParsedTaskCancelRequest{Version: entity.KnowledgeContentVersion, TaskID: strings.Repeat("f", 32), Request: entity.KnowledgeParsedCancelRequest{ExpectedTaskRevision: 1, IdempotencyKey: "cancel"}},
	} {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		c, _ := contentTestContext(string(raw))
		var ok bool
		switch value.(type) {
		case contentPolicyRequest:
			_, ok = decodeContent[contentPolicyRequest](c)
		case contentParsedSnapshotRequest:
			_, ok = decodeContent[contentParsedSnapshotRequest](c)
		case contentParsedSourceRequest:
			_, ok = decodeContent[contentParsedSourceRequest](c)
		case contentParsedTaskGetRequest:
			_, ok = decodeContent[contentParsedTaskGetRequest](c)
		case contentParsedTaskCancelRequest:
			_, ok = decodeContent[contentParsedTaskCancelRequest](c)
		}
		if !ok {
			t.Fatalf("valid owning request rejected: %T", value)
		}
	}
}

func TestKnowledgeParsedOperationErrorsPreserveStatusAndKnownOutcome(t *testing.T) {
	c, recorder := contentTestContext("")
	c.Set("content_operation", "parsed-task-create")
	id := strings.Repeat("e", 64)
	contentHandlerResponse(c, 200, "must-not-escape", &dataset.DocumentOperationError{Status: 429, Code: "capacity_exceeded", OutcomeKnown: true, RejectionID: &id})
	if recorder.Code != 429 || !strings.Contains(recorder.Body.String(), `"outcome_known":true`) || strings.Contains(recorder.Body.String(), "must-not-escape") {
		t.Fatal(recorder.Code, recorder.Body.String())
	}
	c, recorder = contentTestContext("")
	c.Set("content_operation", "parsed-task-create")
	contentHandlerResponse(c, 200, nil, &dataset.DocumentOperationError{Status: 409, Code: "private SQL diagnostic"})
	if recorder.Code != 503 || strings.Contains(recorder.Body.String(), "private SQL") {
		t.Fatal("private error escaped controlled admission response")
	}
}

func TestKnowledgeParsedMultipartAdmissionIsBoundedAndStrict(t *testing.T) {
	for _, invalid := range []string{"", "unknown_form", "duplicate_file", "forged_owner", "duplicate_json", "oversize"} {
		t.Run(invalid, func(t *testing.T) {
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			control := `{"contract_version":"0.2.0-candidate","request":{"expected_policy_version":1,"expected_content_policy_version":2,"confirmed":true}}`
			if invalid == "forged_owner" {
				control = strings.Replace(control, `"confirmed":true`, `"confirmed":true,"owner_id":"forged"`, 1)
			}
			if invalid == "duplicate_json" {
				control = strings.Replace(control, `"confirmed":true`, `"confirmed":true,"confirmed":false`, 1)
			}
			if err := writer.WriteField("request", control); err != nil {
				t.Fatal(err)
			}
			if invalid == "unknown_form" {
				if err := writer.WriteField("owner_id", "forged"); err != nil {
					t.Fatal(err)
				}
			}
			count := 1
			if invalid == "duplicate_file" {
				count = 2
			}
			for i := 0; i < count; i++ {
				part, err := writer.CreateFormFile("file", "synthetic.xlsx")
				if err != nil {
					t.Fatal(err)
				}
				input := []byte("synthetic parser ingress bytes")
				if invalid == "oversize" {
					input = bytes.Repeat([]byte{'a'}, int(entity.KnowledgeMaxSourceBytes)+65537)
				}
				if _, err := part.Write(input); err != nil {
					t.Fatal(err)
				}
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			c, recorder := contentTestContext("")
			c.Request = httptest.NewRequest("POST", "/", &body)
			c.Request.Header.Set("Content-Type", writer.FormDataContentType())
			request, file, cleanup, ok := decodeContentUpload(c)
			defer cleanup()
			if ok != (invalid == "") {
				t.Fatalf("unexpected multipart admission: invalid=%s ok=%t status=%d", invalid, ok, recorder.Code)
			}
			if ok {
				opened, err := file.Open()
				if err != nil {
					t.Fatal(err)
				}
				data, err := io.ReadAll(opened)
				opened.Close()
				if err != nil || string(data) != "synthetic parser ingress bytes" || request.Version != entity.KnowledgeContentVersion {
					t.Fatal("multipart bytes/control changed", err)
				}
			}
		})
	}
}

func TestKnowledgeParsedMultipartAPITokenDeniedBeforeBodyRead(t *testing.T) {
	c, recorder := contentTestContext("must-not-be-parsed")
	c.Set("auth_via_api_token", true)
	(&DatasetsHandler{}).KnowledgeContent("parsed-source-upload")(c)
	if recorder.Code != 401 {
		t.Fatal("API token reached native upload ingress", recorder.Code)
	}
}
