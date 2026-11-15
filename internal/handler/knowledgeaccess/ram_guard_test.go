package knowledgeaccess

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRAMSourceRejectsRegularFilesAndURLSources(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-credential-file")
	if os.WriteFile(file, []byte("INERT_UNIT_VALUE_NOT_A_SESSION"), 0600) != nil {
		t.Fatal("unit fixture write failed")
	}
	for _, name := range []string{file, "", "https://example.invalid/session", "http://127.0.0.1:18080/session"} {
		if source, err := openRAMPipe(name); err != errScope {
			if source != nil {
				_ = source.Close()
			}
			t.Fatal("non-pipe input admitted")
		}
	}
}

func TestRAMSourceRefusesMissingFingerprintWithoutReading(t *testing.T) {
	reads := 0
	source := &countedReader{read: func([]byte) (int, error) { reads++; return 0, io.EOF }}
	if _, err := loadRun(source, "", nil); err != errScope || reads != 0 {
		t.Fatal("missing public fingerprint reached RAM input")
	}
	if _, err := loadRun(bytes.NewReader(bytes.Repeat([]byte("x"), maxRAMInput+1)), strings.Repeat("a", 64), nil); err != errScope {
		t.Fatal("oversize RAM input admitted")
	}
}

type countedReader struct{ read func([]byte) (int, error) }

func (r *countedReader) Read(value []byte) (int, error) { return r.read(value) }

func TestSecretMarkersIncludeBearerValueWithoutExposingIt(t *testing.T) {
	c := &accessClient{auth: map[string]string{"owner": "Bearer UNIT_INERT_VALUE"}}
	markers := c.secretMarkers()
	found := false
	for _, marker := range markers {
		if marker == "UNIT_INERT_VALUE" {
			found = true
		}
	}
	if !found {
		t.Fatal("bare bearer echo would not be detected")
	}
	value, err := strictJSON([]byte(`{"note":"\u0055NIT_INERT_VALUE"}`))
	if err != nil || noEcho(value, markers) {
		t.Fatal("JSON-escaped authentication echo was not detected")
	}
}
