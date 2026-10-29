package dataset

import (
	"runtime"

	"ragflow/internal/entity"
	parser "ragflow/internal/parser/parser"
)

func parsedLocalProfile(format string) (*entity.KnowledgeParsedProfile, error) {
	if !parser.KnowledgeParsedLocalFormat(format) {
		return nil, contentError(400, "invalid_request")
	}
	build := documentBinarySHA()
	if build == "" {
		return nil, contentError(503, "unavailable")
	}
	digest, err := entity.KnowledgeLifecycleDigest(map[string]any{"kind": "parsed-local-producer/0.2.0", "profile_id": parser.KnowledgeParsedLocalProfile, "format": format, "executable_sha256": build, "normalizer": "typed-parse-result/1", "remote_images": false, "email_attachments": false, "archive_expanded_limit": int64(32 << 20)})
	if err != nil {
		return nil, err
	}
	return &entity.KnowledgeParsedProfile{ProfileID: parser.KnowledgeParsedLocalProfile, Format: format, ProducerSHA256: digest, RuntimeReady: runtime.GOOS == "linux" && parser.DocumentStructureChildAvailable()}, nil
}

func KnowledgeParsedProfiles() ([]entity.KnowledgeParsedProfile, error) {
	profiles := []entity.KnowledgeParsedProfile{}
	for _, capability := range parser.RegisteredParserCapabilities() {
		format := string(capability.FileType)
		if !parser.KnowledgeParsedLocalFormat(format) {
			continue
		}
		profile, err := parsedLocalProfile(format)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, *profile)
	}
	return profiles, nil
}
