package entity

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
)

const KnowledgeLifecycleContractVersion = "0.7.0"
const KnowledgeMaxLifecycleVersion int64 = 9007199254740991
const KnowledgeLifecycleJSONBytes = 32768

//go:embed knowledge_lifecycle_capabilities.json
var KnowledgeLifecycleCapabilitiesJSON []byte

type KnowledgeLifecycleSnapshot struct {
	SourceRevisionID  string                           `json:"source_revision_id"`
	SourceInputSHA256 string                           `json:"source_input_sha256"`
	LifecycleVersion  int64                            `json:"lifecycle_version"`
	State             string                           `json:"state"`
	RelativePath      string                           `json:"relative_path"`
	Classification    KnowledgeDirectoryClassification `json:"classification"`
	SHA256            string                           `json:"sha256"`
	ByteLength        int64                            `json:"byte_length"`
}

type KnowledgeLifecycleActionCapability struct {
	Supported bool    `json:"supported"`
	Allowed   bool    `json:"allowed"`
	Reason    *string `json:"reason"`
}

type KnowledgeLifecycleDetail struct {
	OwnerID            string                                        `json:"owner_id"`
	DomainID           string                                        `json:"domain_id"`
	SourceID           string                                        `json:"source_id"`
	NativeFileID       string                                        `json:"native_file_id"`
	RegistrationID     *string                                       `json:"registration_id"`
	SourceKind         *string                                       `json:"source_kind"`
	RepresentationKind string                                        `json:"representation_kind"`
	PolicyVersion      int64                                         `json:"policy_version"`
	Current            KnowledgeLifecycleSnapshot                    `json:"current"`
	Actions            map[string]KnowledgeLifecycleActionCapability `json:"actions"`
}

type KnowledgeLifecyclePage struct {
	Items      []KnowledgeLifecycleDetail `json:"items"`
	NextCursor *string                    `json:"next_cursor"`
}

type KnowledgeLifecycleCommand struct {
	IdempotencyKey           string `json:"idempotency_key"`
	ExpectedSourceRevisionID string `json:"expected_source_revision_id"`
	ExpectedLifecycleVersion int64  `json:"expected_lifecycle_version"`
	ExpectedPolicyVersion    int64  `json:"expected_policy_version"`
	Confirmed                bool   `json:"confirmed"`
}

type KnowledgeLifecycleReclassify struct {
	KnowledgeLifecycleCommand
	Classification KnowledgeDirectoryClassification `json:"classification"`
}

type KnowledgeLifecycleMove struct {
	KnowledgeLifecycleCommand
	RelativePath string `json:"relative_path"`
}

type KnowledgeLifecycleReceipt struct {
	CommandID      string                     `json:"command_id"`
	OwnerID        string                     `json:"owner_id"`
	DomainID       string                     `json:"domain_id"`
	SourceID       string                     `json:"source_id"`
	Action         string                     `json:"action"`
	IdempotencyKey string                     `json:"idempotency_key"`
	RequestSHA256  string                     `json:"request_sha256"`
	Request        json.RawMessage            `json:"request"`
	Result         string                     `json:"result"`
	Before         KnowledgeLifecycleSnapshot `json:"before"`
	After          KnowledgeLifecycleSnapshot `json:"after"`
	PolicyVersion  int64                      `json:"policy_version"`
	CreatedAt      string                     `json:"created_at"`
	ModelCalls     int                        `json:"model_calls"`
}

type KnowledgeLifecycleTarget struct {
	DomainID                string  `json:"domain_id"`
	SourceID                string  `json:"source_id"`
	RegistrationID          *string `json:"registration_id"`
	CurrentSourceRevisionID string  `json:"current_source_revision_id"`
	LifecycleVersion        int64   `json:"lifecycle_version"`
	PolicyVersion           int64   `json:"policy_version"`
	RelativePath            string  `json:"relative_path"`
	Allowed                 bool    `json:"allowed"`
	Reason                  *string `json:"reason"`
	Reserved                bool    `json:"reserved"`
}

// KnowledgeLifecycleDigest encodes the complete approved typed-LP value. It
// shares the scalar codec with web digests, without changing their projections.
func KnowledgeLifecycleDigest(value any) (string, error) {
	decoded, err := webDigestValue(value)
	if err != nil {
		return "", err
	}
	var encoded bytes.Buffer
	if err := webEncodeValue(&encoded, decoded, 0); err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded.Bytes())
	return hex.EncodeToString(digest[:]), nil
}
