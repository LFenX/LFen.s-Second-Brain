package entity

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

func knowledgeDocumentObject(value any) map[string]any {
	object, _ := value.(map[string]any)
	return object
}

func knowledgeDocumentChild(value map[string]any, key string) map[string]any {
	return knowledgeDocumentObject(value[key])
}

func knowledgeDocumentInt(value any) int64 {
	if number, ok := value.(json.Number); ok {
		result, _ := number.Int64()
		return result
	}
	return 0
}

func knowledgeDocumentSHA(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func knowledgeDocumentProjectionDigest(value any) string {
	result, _ := KnowledgeLifecycleDigest(value)
	return result
}

func knowledgeDocumentRequestDigestMap(record map[string]any, operation string) string {
	original := knowledgeDocumentChild(record, "original_request")
	request := make(map[string]any, len(original))
	for key, value := range original {
		if key != "idempotency_key" {
			request[key] = value
		}
	}
	projection := map[string]any{
		"kind":     "knowledge-document-" + operation + "/0.1.0",
		"owner_id": record["owner_id"], "domain_id": record["domain_id"], "request": request,
	}
	if operation == "cancel" {
		projection["task_id"] = knowledgeDocumentChild(record, "target")["task_id"]
	}
	return knowledgeDocumentProjectionDigest(projection)
}

func knowledgeDocumentRepresentationProjection(value map[string]any) map[string]any {
	keys := []string{
		"owner_id", "domain_id", "original", "source_snapshot", "foundation_policy_version", "document_policy_version",
		"format", "profile_id", "producer_sha256", "quality", "reading_format", "reading_text_sha256",
		"reading_byte_length", "structure_manifest_sha256",
	}
	projection := map[string]any{"kind": "knowledge-document-representation/0.1.0"}
	for _, key := range keys {
		projection[key] = value[key]
	}
	return projection
}

func knowledgeDocumentValidatePredicates(schema map[string]any, value any) error {
	for keyword, enabled := range schema {
		if enabled != true {
			continue
		}
		if !knowledgeDocumentPredicate(keyword, value) {
			return fmt.Errorf("document relational constraint failed: %s", keyword)
		}
	}
	return nil
}

func knowledgeDocumentPredicate(keyword string, value any) bool {
	object := knowledgeDocumentObject(value)
	switch keyword {
	case "x-profile-bindings":
		configuration, producer := object["configuration"], knowledgeDocumentChild(object, "producer")
		return producer["configuration_sha256"] == knowledgeDocumentProjectionDigest(map[string]any{"kind": "knowledge-document-configuration/0.1.0", "configuration": configuration}) &&
			object["producer_sha256"] == knowledgeDocumentProjectionDigest(map[string]any{"kind": "knowledge-document-producer/0.1.0", "producer": producer})
	case "x-capabilities-ready":
		if object["runtime_ready"] != true {
			return true
		}
		profiles, _ := object["profiles"].([]any)
		for _, raw := range profiles {
			if knowledgeDocumentObject(raw)["runtime_ready"] != true {
				return false
			}
		}
		return true
	case "x-policy-receipt":
		request, before, after := knowledgeDocumentChild(object, "original_request"), knowledgeDocumentChild(object, "before"), knowledgeDocumentChild(object, "after")
		if object["idempotency_key"] != request["idempotency_key"] ||
			knowledgeDocumentChild(object, "target")["domain_id"] != object["domain_id"] ||
			!knowledgeDocumentEqual(request["expected_document_policy_version"], before["document_policy_version"]) ||
			object["request_sha256"] != knowledgeDocumentRequestDigestMap(object, "policy") {
			return false
		}
		if object["outcome"] == "unchanged" {
			return knowledgeDocumentEqual(before, after) && request["enabled"] == before["enabled"]
		}
		return knowledgeDocumentInt(after["document_policy_version"]) == knowledgeDocumentInt(before["document_policy_version"])+1 &&
			after["enabled"] == request["enabled"] && before["enabled"] != after["enabled"] &&
			knowledgeDocumentInt(after["document_policy_version"]) <= 9007199254740991
	case "x-progress-range":
		return object["total_units"] == nil || knowledgeDocumentInt(object["completed_units"]) <= knowledgeDocumentInt(object["total_units"])
	case "x-reading-range":
		start, end := knowledgeDocumentInt(object["start_byte"]), knowledgeDocumentInt(object["end_byte"])
		return start <= end && (start != end || object["text_sha256"] == knowledgeDocumentSHA(""))
	case "x-task-bindings":
		request, original, snapshot := knowledgeDocumentChild(object, "original_request"), knowledgeDocumentChild(object, "original"), knowledgeDocumentChild(object, "source_snapshot")
		target := knowledgeDocumentChild(request, "target")
		created, err1 := time.Parse(time.RFC3339Nano, fmt.Sprint(object["created_at"]))
		updated, err2 := time.Parse(time.RFC3339Nano, fmt.Sprint(object["updated_at"]))
		deadline, err3 := time.Parse(time.RFC3339Nano, fmt.Sprint(object["deadline_at"]))
		if err1 != nil || err2 != nil || err3 != nil || deadline.Sub(created) != 120*time.Second || updated.Before(created) {
			return false
		}
		if object["completed_at"] != nil {
			completed, err := time.Parse(time.RFC3339Nano, fmt.Sprint(object["completed_at"]))
			if err != nil || completed.Before(created) {
				return false
			}
		}
		return original["domain_id"] == object["domain_id"] && target["source_id"] == original["source_id"] &&
			target["source_revision_id"] == original["source_revision_id"] && target["original_sha256"] == original["sha256"] &&
			knowledgeDocumentEqual(target["expected_lifecycle_version"], snapshot["lifecycle_version"]) &&
			request["expected_producer_sha256"] == object["producer_sha256"] &&
			object["request_sha256"] == knowledgeDocumentRequestDigestMap(object, "task")
	case "x-cancel-receipt":
		request := knowledgeDocumentChild(object, "original_request")
		if object["idempotency_key"] != request["idempotency_key"] || object["request_sha256"] != knowledgeDocumentRequestDigestMap(object, "cancel") {
			return false
		}
		if object["outcome"] == "requested" {
			return knowledgeDocumentInt(object["task_revision"]) == knowledgeDocumentInt(request["expected_task_revision"])+1
		}
		return knowledgeDocumentEqual(object["task_revision"], request["expected_task_revision"]) && object["before_state"] == object["after_state"]
	case "x-rejection-bindings":
		request := knowledgeDocumentChild(object, "original_request")
		operation, _ := object["operation"].(string)
		if object["idempotency_key"] != request["idempotency_key"] || object["request_sha256"] != knowledgeDocumentRequestDigestMap(object, operation) {
			return false
		}
		switch operation {
		case "task":
			return knowledgeDocumentEqual(object["target"], request["target"])
		case "policy":
			return knowledgeDocumentChild(object, "target")["domain_id"] == object["domain_id"]
		case "cancel":
			return true
		default:
			return false
		}
	case "x-head-current":
		return object["currentness"] != "current" || object["active_source_revision_id"] == object["current_source_revision_id"]
	case "x-node-payload":
		return knowledgeDocumentPayloadValid(object)
	case "x-document-node":
		payloadSHA := knowledgeDocumentProjectionDigest(map[string]any{"kind": "knowledge-document-node-payload/0.1.0", "payload": object["payload"]})
		return object["node_payload_sha256"] == payloadSHA &&
			object["document_node_id"] == knowledgeDocumentProjectionDigest(map[string]any{
				"kind": "knowledge-document-node/0.1.0", "representation_id": object["representation_id"],
				"source_revision_id": object["source_revision_id"], "node_payload_sha256": object["node_payload_sha256"],
			})
	case "x-representation":
		producer, original := knowledgeDocumentChild(object, "producer"), knowledgeDocumentChild(object, "original")
		return original["domain_id"] == object["domain_id"] &&
			producer["configuration_sha256"] == knowledgeDocumentProjectionDigest(map[string]any{"kind": "knowledge-document-configuration/0.1.0", "configuration": object["configuration"]}) &&
			object["producer_sha256"] == knowledgeDocumentProjectionDigest(map[string]any{"kind": "knowledge-document-producer/0.1.0", "producer": producer}) &&
			object["representation_id"] == knowledgeDocumentProjectionDigest(knowledgeDocumentRepresentationProjection(object)) &&
			knowledgeDocumentInt(object["unsupported_block_count"]) <= knowledgeDocumentInt(object["node_count"]) &&
			(knowledgeDocumentInt(object["reading_byte_length"]) != 0 || object["reading_text_sha256"] == knowledgeDocumentSHA(""))
	case "x-node-page":
		items, _ := object["items"].([]any)
		seen := make(map[any]bool, len(items))
		var previous map[string]any
		for _, raw := range items {
			item := knowledgeDocumentObject(raw)
			if item["parent_id"] != object["parent_id"] || item["representation_id"] != object["representation_id"] ||
				item["source_id"] != object["source_id"] || item["source_revision_id"] != object["source_revision_id"] || seen[item["document_node_id"]] {
				return false
			}
			if previous != nil {
				left := knowledgeDocumentInt(knowledgeDocumentChild(previous, "payload")["ordinal"])
				right := knowledgeDocumentInt(knowledgeDocumentChild(item, "payload")["ordinal"])
				if left > right || left == right && fmt.Sprint(previous["document_node_id"]) >= fmt.Sprint(item["document_node_id"]) {
					return false
				}
			}
			seen[item["document_node_id"]] = true
			previous = item
		}
		return true
	case "x-content-window":
		text, ok := object["text"].(string)
		if !ok {
			return false
		}
		start, end := knowledgeDocumentInt(object["start_byte"]), knowledgeDocumentInt(object["end_byte"])
		if end < start || int64(len(text)) != end-start || end-start > 65536 || object["fragment_sha256"] != knowledgeDocumentSHA(text) {
			return false
		}
		if object["complete"] == false {
			return end > start && knowledgeDocumentEqual(object["next_start_byte"], object["end_byte"])
		}
		return true
	case "x-content-query":
		if _, exists := object["end_byte"]; !exists {
			return true
		}
		if _, exists := object["start_byte"]; !exists {
			return false
		}
		start, end := knowledgeDocumentInt(object["start_byte"]), knowledgeDocumentInt(object["end_byte"])
		return start <= end && end-start <= 65536
	default:
		return true
	}
}

func knowledgeDocumentPayloadValid(payload map[string]any) bool {
	rangeValue := knowledgeDocumentChild(payload, "reading_range")
	start, end := knowledgeDocumentInt(rangeValue["start_byte"]), knowledgeDocumentInt(rangeValue["end_byte"])
	status := payload["text_status"]
	if status == "available" && end <= start {
		return false
	}
	if (status == "empty" || status == "unsupported" || status == "unsupported_or_requires_ocr") && start != end {
		return false
	}
	if payload["own_reading_range"] != nil {
		own := knowledgeDocumentChild(payload, "own_reading_range")
		if knowledgeDocumentInt(own["start_byte"]) < start || knowledgeDocumentInt(own["end_byte"]) > end {
			return false
		}
	}
	attributes, locator, position := knowledgeDocumentChild(payload, "attributes"), knowledgeDocumentChild(payload, "locator"), knowledgeDocumentChild(payload, "original_position")
	switch payload["kind"] {
	case "page":
		return knowledgeDocumentEqual(attributes["page_number"], locator["page_number"]) && knowledgeDocumentEqual(position["page_number"], locator["page_number"])
	case "slide":
		return knowledgeDocumentEqual(attributes["slide_number"], position["slide_number"])
	case "media":
		if status != "unsupported" || payload["text_reason"] != "media_not_ocr" {
			return false
		}
		return attributes["placement"] != "inline" || knowledgeDocumentEqual(attributes["inline_anchor_byte"], rangeValue["start_byte"])
	case "unsupported_block":
		return status == "unsupported" && payload["text_reason"] == "unsupported_source_block"
	case "passage":
		parent := knowledgeDocumentChild(payload, "parent_ref")
		return end-start <= 8192 && knowledgeDocumentEqual(attributes["chunk_index"], locator["chunk_index"]) &&
			knowledgeDocumentEqual(parent["locator"], locator["parent_locator"])
	default:
		return true
	}
}
