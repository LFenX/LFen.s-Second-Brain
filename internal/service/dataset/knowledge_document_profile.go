package dataset

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"runtime"
	"sync"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	parser "ragflow/internal/parser/parser"
)

var documentBuildOnce sync.Once
var documentBuildSHA string

// The producer commits to the actual linked server, including the native PDF
// engine and source adapter. Synthetic contract hashes are never runtime IDs.
func documentBinarySHA() string {
	documentBuildOnce.Do(func() {
		name, err := os.Executable()
		if err != nil {
			return
		}
		if runtime.GOOS == "linux" {
			name = "/proc/self/exe"
		}
		f, err := os.Open(name)
		if err != nil {
			return
		}
		defer f.Close()
		h := sha256.New()
		if _, err := io.Copy(h, f); err == nil {
			documentBuildSHA = hex.EncodeToString(h.Sum(nil))
		}
	})
	return documentBuildSHA
}

// ParseKnowledgeDocumentChild is the private-process entry adapter used before
// server bootstrap. It verifies this executable's actual producer and performs
// only bounded local parsing, without creating services, queues or models.
func ParseKnowledgeDocumentChild(ctx context.Context, control parser.DocumentChildControl, input []byte) (json.RawMessage, error) {
	profile, err := documentProfile(control.ProfileID)
	if err != nil || !profile.RuntimeReady || profile.Format != control.Format || profile.ProducerSHA256 != control.ProducerSHA256 {
		return nil, &parser.DocumentStructureError{Code: "producer_changed"}
	}
	_, encoded, err := parser.BuildDocumentStructure(ctx, control.Format, input)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

func documentProfile(id string) (*entity.KnowledgeDocumentProfile, error) {
	format, version := "", "go_ooxml/0.1.0"
	switch id {
	case entity.KnowledgeDocumentPDFProfile:
		format, version = "pdf", "v0.3.73"
	case entity.KnowledgeDocumentPPTXProfile:
		format = "pptx"
	case entity.KnowledgeDocumentDOCXProfile:
		format = "docx"
	default:
		return nil, documentError(400, "invalid_request")
	}
	build := documentBinarySHA()
	if build == "" {
		return nil, documentError(503, "unavailable")
	}
	profile, err := entity.KnowledgeDocumentBuildProfile(id, version, build, "document-source/0.1.0", runtime.GOOS == "linux" && parser.DocumentStructureRuntimeReady(format) && parser.DocumentStructureChildAvailable())
	return &profile, err
}

func KnowledgeDocumentCapabilities() (*entity.KnowledgeDocumentCapabilities, error) {
	profiles := []entity.KnowledgeDocumentProfile{}
	ready := dao.KnowledgeDocumentRuntimeReady()
	for _, id := range []string{entity.KnowledgeDocumentPDFProfile, entity.KnowledgeDocumentPPTXProfile, entity.KnowledgeDocumentDOCXProfile} {
		p, err := documentProfile(id)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, *p)
		ready = ready && p.RuntimeReady
	}
	value, err := entity.KnowledgeDocumentBuildCapabilities(profiles, ready)
	return &value, err
}
