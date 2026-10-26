//go:build !linux

package parser

import (
	"context"
	"encoding/json"
)

func DocumentStructureChildAvailable() bool { return false }

func RunDocumentStructureChild(context.Context, []byte, DocumentChildControl, func(DocumentChildOwnership) error) (json.RawMessage, error) {
	return nil, ErrDocumentChildUnavailable
}

func RecoverDocumentStructureChild(context.Context, DocumentChildOwnership) error {
	return ErrDocumentChildUnavailable
}

func RecoverDocumentStructureAttempt(context.Context, string, int) error {
	return ErrDocumentChildUnavailable
}

func DocumentStructureChildMain(context.Context, DocumentChildParser) int { return 78 }
