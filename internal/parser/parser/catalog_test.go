package parser

import (
	"testing"

	"ragflow/internal/utility"
)

func TestRegisteredParserCapabilitiesPreserveExtractionBoundary(t *testing.T) {
	capabilities := RegisteredParserCapabilities()
	foundAudio := false
	for _, capability := range capabilities {
		if capability.FileType == utility.FileTypeVIDEO {
			t.Fatal("component-only video processing was advertised as a base parser")
		}
		if capability.FileType != utility.FileTypeAURAL {
			continue
		}
		foundAudio = true
		if capability.BaseResult != "configuration_only" {
			t.Fatal("audio configuration must not advertise extracted text")
		}
		p, err := GetParser(capability.FileType)
		if err != nil {
			t.Fatal(err)
		}
		result := p.ParseWithResult(t.Context(), "synthetic.mp3", []byte{1, 2, 3})
		if result.Err != nil || result.Text != "" || len(result.JSON) != 0 || result.Markdown != "" {
			t.Fatal("audio base parser extraction boundary changed")
		}
	}
	if !foundAudio {
		t.Fatal("registered audio validation is missing")
	}
	capabilities[0].FileType = utility.FileTypeOTHER
	if RegisteredParserCapabilities()[0].FileType == utility.FileTypeOTHER {
		t.Fatal("a consumer changed the shared parser registry")
	}
	if p, err := GetParser(utility.FileTypeVIDEO); err == nil || p != nil {
		t.Fatal("video must remain in its explicit component path")
	}
}
