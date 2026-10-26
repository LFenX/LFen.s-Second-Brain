package entity

import (
	"bytes"
	"encoding/json"
	"fmt"
)

func knowledgeDocumentRequestValue(value any) (map[string]any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var request map[string]any
	if err := decoder.Decode(&request); err != nil {
		return nil, err
	}
	delete(request, "idempotency_key")
	return request, nil
}

func KnowledgeDocumentTaskDigest(owner, domain string, request KnowledgeDocumentTaskRequest) (string, error) {
	if !knowledgeDocumentHex(owner, 32) || !knowledgeDocumentHex(domain, 32) {
		return "", fmt.Errorf("invalid document task scope")
	}
	value, err := knowledgeDocumentRequestValue(request)
	if err != nil {
		return "", err
	}
	return KnowledgeLifecycleDigest(map[string]any{
		"kind": "knowledge-document-task/0.1.0", "owner_id": owner, "domain_id": domain, "request": value,
	})
}

func KnowledgeDocumentPolicyDigest(owner, domain string, request KnowledgeDocumentPolicyChangeRequest) (string, error) {
	if !knowledgeDocumentHex(owner, 32) || !knowledgeDocumentHex(domain, 32) {
		return "", fmt.Errorf("invalid document policy scope")
	}
	value, err := knowledgeDocumentRequestValue(request)
	if err != nil {
		return "", err
	}
	return KnowledgeLifecycleDigest(map[string]any{
		"kind": "knowledge-document-policy/0.1.0", "owner_id": owner, "domain_id": domain, "request": value,
	})
}

func KnowledgeDocumentCancelDigest(owner, domain, taskID string, request KnowledgeDocumentCancelRequest) (string, error) {
	if !knowledgeDocumentHex(owner, 32) || !knowledgeDocumentHex(domain, 32) || !knowledgeDocumentHex(taskID, 32) {
		return "", fmt.Errorf("invalid document cancel scope")
	}
	value, err := knowledgeDocumentRequestValue(request)
	if err != nil {
		return "", err
	}
	return KnowledgeLifecycleDigest(map[string]any{
		"kind": "knowledge-document-cancel/0.1.0", "owner_id": owner, "domain_id": domain, "task_id": taskID, "request": value,
	})
}

func KnowledgeDocumentConfigurationDigest(configuration KnowledgeDocumentConfiguration) (string, error) {
	return KnowledgeLifecycleDigest(map[string]any{"kind": "knowledge-document-configuration/0.1.0", "configuration": configuration})
}

func KnowledgeDocumentProducerDigest(producer KnowledgeDocumentProducer) (string, error) {
	return KnowledgeLifecycleDigest(map[string]any{"kind": "knowledge-document-producer/0.1.0", "producer": producer})
}

func KnowledgeDocumentPayloadDigest(payload KnowledgeDocumentNodePayload) (string, error) {
	return KnowledgeLifecycleDigest(map[string]any{"kind": "knowledge-document-node-payload/0.1.0", "payload": payload})
}

func KnowledgeDocumentManifestDigest(nodes []KnowledgeDocumentManifestEntry) (string, error) {
	if len(nodes) < 1 || len(nodes) > 8192 {
		return "", fmt.Errorf("invalid document manifest size")
	}
	return KnowledgeLifecycleDigest(map[string]any{"kind": "knowledge-document-structure-manifest/0.1.0", "nodes": nodes})
}

// KnowledgeDocumentRepresentationDigest excludes all derived node IDs and time.
// The service computes this before assigning root_document_node_id.
func KnowledgeDocumentRepresentationDigest(value KnowledgeDocumentRepresentation) (string, error) {
	if !knowledgeDocumentHex(value.OwnerID, 32) || !knowledgeDocumentHex(value.DomainID, 32) ||
		value.Original.DomainID != value.DomainID ||
		!knowledgeDocumentHex(value.ProducerSHA256, 64) || !knowledgeDocumentHex(value.ReadingTextSHA256, 64) ||
		!knowledgeDocumentHex(value.StructureManifestSHA256, 64) ||
		value.FoundationPolicyVersion < 1 || value.FoundationPolicyVersion > KnowledgeMaxLifecycleVersion ||
		value.DocumentPolicyVersion < 1 || value.DocumentPolicyVersion > KnowledgeMaxLifecycleVersion ||
		value.ReadingByteLength < 0 || value.ReadingByteLength > 4194304 ||
		value.Quality != "complete" || value.ReadingFormat != KnowledgeDocumentReadingFormat {
		return "", fmt.Errorf("invalid document representation identity input")
	}
	switch value.ProfileID {
	case KnowledgeDocumentPDFProfile:
		if value.Format != "pdf" {
			return "", fmt.Errorf("document profile and format differ")
		}
	case KnowledgeDocumentPPTXProfile:
		if value.Format != "pptx" {
			return "", fmt.Errorf("document profile and format differ")
		}
	case KnowledgeDocumentDOCXProfile:
		if value.Format != "docx" {
			return "", fmt.Errorf("document profile and format differ")
		}
	default:
		return "", fmt.Errorf("unknown document representation profile")
	}
	return KnowledgeLifecycleDigest(map[string]any{
		"kind":                      "knowledge-document-representation/0.1.0",
		"owner_id":                  value.OwnerID,
		"domain_id":                 value.DomainID,
		"original":                  value.Original,
		"source_snapshot":           value.SourceSnapshot,
		"foundation_policy_version": value.FoundationPolicyVersion,
		"document_policy_version":   value.DocumentPolicyVersion,
		"format":                    value.Format,
		"profile_id":                value.ProfileID,
		"producer_sha256":           value.ProducerSHA256,
		"quality":                   value.Quality,
		"reading_format":            value.ReadingFormat,
		"reading_text_sha256":       value.ReadingTextSHA256,
		"reading_byte_length":       value.ReadingByteLength,
		"structure_manifest_sha256": value.StructureManifestSHA256,
	})
}

func KnowledgeDocumentNodeDigest(representationID, sourceRevisionID, payloadSHA string) (string, error) {
	if !knowledgeDocumentHex(representationID, 64) || !knowledgeDocumentHex(sourceRevisionID, 32) || !knowledgeDocumentHex(payloadSHA, 64) {
		return "", fmt.Errorf("invalid document node identity input")
	}
	return KnowledgeLifecycleDigest(map[string]any{
		"kind": "knowledge-document-node/0.1.0", "representation_id": representationID,
		"source_revision_id": sourceRevisionID, "node_payload_sha256": payloadSHA,
	})
}

// KnowledgeDocumentAttributes preserves explicit null fields in typed attributes.
// The complete payload codec then checks the attributes against its node kind.
func KnowledgeDocumentAttributes(value any) (json.RawMessage, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoded, err := knowledgeDocumentJSON(raw)
	if err != nil {
		return nil, err
	}
	if _, ok := decoded.(map[string]any); !ok {
		return nil, fmt.Errorf("document attributes require an object")
	}
	return json.RawMessage(raw), nil
}

func KnowledgeDocumentConfigurationForProfile(profileID string) (KnowledgeDocumentConfiguration, error) {
	extraction := "office_ooxml_source"
	switch profileID {
	case KnowledgeDocumentPDFProfile:
		extraction = "pdf_plain_text"
	case KnowledgeDocumentPPTXProfile, KnowledgeDocumentDOCXProfile:
	default:
		return KnowledgeDocumentConfiguration{}, fmt.Errorf("unknown document profile")
	}
	return KnowledgeDocumentConfiguration{
		Extraction: extraction, ReadingFormat: KnowledgeDocumentReadingFormat,
		NormalizeDOCXLineSpacing: false, TrimText: false, RemoteResources: false,
		OCR: false, Models: false, MediaOutput: "metadata_only",
	}, nil
}

// KnowledgeDocumentBuildProducer requires the actual engine/build/adapter inputs.
// The caller owns provenance; this helper has no placeholder or synthetic default.
func KnowledgeDocumentBuildProducer(profileID, engineVersion, engineBuildSHA256, adapterVersion string) (KnowledgeDocumentProducer, error) {
	configuration, err := KnowledgeDocumentConfigurationForProfile(profileID)
	if err != nil {
		return KnowledgeDocumentProducer{}, err
	}
	configSHA, err := KnowledgeDocumentConfigurationDigest(configuration)
	if err != nil {
		return KnowledgeDocumentProducer{}, err
	}
	format, engine := "pdf", "pdf_oxide"
	switch profileID {
	case KnowledgeDocumentPPTXProfile:
		format, engine = "pptx", "go_ooxml"
	case KnowledgeDocumentDOCXProfile:
		format, engine = "docx", "go_ooxml"
	}
	value := KnowledgeDocumentProducer{
		ProfileID: profileID, Format: format, EngineID: engine,
		EngineVersion: engineVersion, EngineBuildSHA256: engineBuildSHA256,
		AdapterVersion: adapterVersion, ReadingFormatVersion: KnowledgeDocumentReadingFormat,
		ConfigurationSHA256: configSHA,
	}
	if _, err := json.Marshal(value); err != nil {
		return KnowledgeDocumentProducer{}, err
	}
	return value, nil
}

func KnowledgeDocumentBuildProfile(profileID, engineVersion, engineBuildSHA256, adapterVersion string, ready bool) (KnowledgeDocumentProfile, error) {
	configuration, err := KnowledgeDocumentConfigurationForProfile(profileID)
	if err != nil {
		return KnowledgeDocumentProfile{}, err
	}
	producer, err := KnowledgeDocumentBuildProducer(profileID, engineVersion, engineBuildSHA256, adapterVersion)
	if err != nil {
		return KnowledgeDocumentProfile{}, err
	}
	digest, err := KnowledgeDocumentProducerDigest(producer)
	if err != nil {
		return KnowledgeDocumentProfile{}, err
	}
	mediaType := "application/pdf"
	switch producer.Format {
	case "pptx":
		mediaType = "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	case "docx":
		mediaType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	}
	return KnowledgeDocumentProfile{
		ProfileID: profileID, Format: producer.Format, MediaType: mediaType,
		Configuration: configuration, Producer: producer, ProducerSHA256: digest, RuntimeReady: ready,
	}, nil
}

func KnowledgeDocumentBuildCapabilities(profiles []KnowledgeDocumentProfile, ready bool) (KnowledgeDocumentCapabilities, error) {
	constants := make(map[string]any)
	properties, _ := knowledgeDocumentSchemas["Limits"]["properties"].(map[string]any)
	for name, raw := range properties {
		property, _ := raw.(map[string]any)
		constants[name] = property["const"]
	}
	encoded, err := json.Marshal(constants)
	if err != nil {
		return KnowledgeDocumentCapabilities{}, err
	}
	var limits KnowledgeDocumentLimits
	if err := json.Unmarshal(encoded, &limits); err != nil {
		return KnowledgeDocumentCapabilities{}, err
	}
	value := KnowledgeDocumentCapabilities{
		ContractVersion: "1.0.0", DocumentStructureContractVersion: KnowledgeDocumentContractVersion,
		RuntimeReady: ready, OwnerOnly: true, Mode: "document_structure_only",
		DocumentPolicyDefaultEnabled: false, CodeStructurePermissionRequired: false,
		AutomaticProcessing: false, LexicalIndex: "not_created", VectorIndex: "not_created",
		ExecutionQuota: KnowledgeDocumentCapabilitiesExecutionQuota{DomainRunning: 1, DomainQueued: 1, SharedWithCode: true},
		Execution: KnowledgeDocumentCapabilitiesExecution{
			Mode: "owned_subprocess", CancelAction: "terminate_owned_process_group_and_wait",
			CancellationComplete: "after_exit_and_cleanup", HardMemoryIsolation: false,
		},
		Profiles: profiles, Limits: limits,
	}
	if _, err := json.Marshal(value); err != nil {
		return KnowledgeDocumentCapabilities{}, err
	}
	return value, nil
}
