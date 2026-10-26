// Copyright 2026 LFen's Second Brain contributors.
// SPDX-License-Identifier: Apache-2.0
package dataset

import (
	"context"
	"errors"
	"io"

	"ragflow/internal/parser/parser"
)

func inspectKnowledgeOriginalOffice(ctx context.Context, source io.ReaderAt, size int64, mediaType string, limits knowledgeOriginalLimits) error {
	_, err := parser.OpenOfficePackage(ctx, source, size, mediaType, parser.OfficePackageLimits{
		Entries: limits.officeEntries, ExpandedBytes: limits.officeExpandedBytes, Ratio: limits.officeRatio,
		XMLBytes: limits.officeXMLBytes, AllXMLBytes: limits.officeAllXMLBytes, DirectoryBytes: limits.officeDirectoryBytes, XMLDepth: limits.officeXMLDepth,
	})
	if errors.Is(err, parser.ErrOfficePackage) || errors.Is(err, parser.ErrOfficePackageBudget) {
		return errOriginalType
	}
	return err
}
