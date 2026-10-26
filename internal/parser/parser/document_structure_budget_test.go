package parser

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
)

type documentZeroReader struct{}

func (documentZeroReader) Read(b []byte) (int, error) { clear(b); return len(b), nil }

func TestDocumentStructureExpandedPackageBudgetIsControlled(t *testing.T) {
	var buf bytes.Buffer
	archive := zip.NewWriter(&buf)
	entry, err := archive.Create("padding.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.CopyN(entry, documentZeroReader{}, 17<<20); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if buf.Len() > 8<<20 {
		t.Fatal("fixture must fit original input budget")
	}
	result, err := BuildDocumentStructure(context.Background(), "docx", buf.Bytes())
	var controlled *DocumentStructureError
	if !errors.As(err, &controlled) || controlled.Code != "budget_exceeded" || len(result.Payloads) != 0 {
		t.Fatalf("expanded package must fail without a partial result: %v", err)
	}
}
