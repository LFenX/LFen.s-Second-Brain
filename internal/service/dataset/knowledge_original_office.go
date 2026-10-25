// Copyright 2026 LFen's Second Brain contributors.
// SPDX-License-Identifier: Apache-2.0

package dataset

import (
	"archive/zip"
	"context"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

// inspectKnowledgeOfficeDirectory bounds the real directory before archive/zip
// allocates its file list. This small-package path excludes ZIP64 and volumes.
func inspectKnowledgeOfficeDirectory(ctx context.Context, source io.ReaderAt, size int64, limits knowledgeOriginalLimits) error {
	if size < 22 {
		return errOriginalType
	}
	var signature [4]byte
	if _, err := source.ReadAt(signature[:], 0); err != nil {
		return err
	}
	if binary.LittleEndian.Uint32(signature[:]) != 0x04034b50 {
		return errOriginalType
	}
	tail := make([]byte, min(size, 22+65535))
	if _, err := source.ReadAt(tail, size-int64(len(tail))); err != nil {
		return err
	}
	end := -1
	for i := len(tail) - 22; i >= 0; i-- {
		if binary.LittleEndian.Uint32(tail[i:]) == 0x06054b50 && i+22+int(binary.LittleEndian.Uint16(tail[i+20:])) == len(tail) {
			end = i
			break
		}
	}
	if end < 0 {
		return errOriginalType
	}
	record := tail[end:]
	entries := int(binary.LittleEndian.Uint16(record[10:]))
	directoryBytes := int64(binary.LittleEndian.Uint32(record[12:]))
	offset := int64(binary.LittleEndian.Uint32(record[16:]))
	endOffset := size - int64(len(tail)) + int64(end)
	if binary.LittleEndian.Uint16(record[4:]) != 0 || binary.LittleEndian.Uint16(record[6:]) != 0 ||
		int(binary.LittleEndian.Uint16(record[8:])) != entries || entries < 1 || entries > limits.officeEntries || entries == 65535 ||
		directoryBytes <= 0 || directoryBytes > limits.officeDirectoryBytes || offset+directoryBytes != endOffset {
		return errOriginalType
	}
	position, count := offset, 0
	for position < endOffset {
		if err := ctx.Err(); err != nil {
			return err
		}
		var header [46]byte
		if endOffset-position < int64(len(header)) {
			return errOriginalType
		}
		if _, err := source.ReadAt(header[:], position); err != nil {
			return err
		}
		if binary.LittleEndian.Uint32(header[:]) != 0x02014b50 || binary.LittleEndian.Uint16(header[34:]) != 0 ||
			binary.LittleEndian.Uint32(header[20:]) == 0xffffffff || binary.LittleEndian.Uint32(header[24:]) == 0xffffffff ||
			binary.LittleEndian.Uint32(header[42:]) == 0xffffffff {
			return errOriginalType
		}
		length := int64(len(header)) + int64(binary.LittleEndian.Uint16(header[28:])) +
			int64(binary.LittleEndian.Uint16(header[30:])) + int64(binary.LittleEndian.Uint16(header[32:]))
		if length > endOffset-position || count >= limits.officeEntries {
			return errOriginalType
		}
		position += length
		count++
	}
	if position != endOffset || count != entries {
		return errOriginalType
	}
	return nil
}

func isKnowledgeOfficePath(name string) bool {
	clean := strings.TrimSuffix(name, "/")
	if clean == "" || len(name) > 512 || !utf8.ValidString(name) || strings.HasPrefix(name, "/") ||
		strings.ContainsAny(name, "\\:") || strings.ContainsFunc(name, unicode.IsControl) || path.Clean(clean) != clean {
		return false
	}
	for _, part := range strings.Split(clean, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func inspectKnowledgeOriginalOffice(ctx context.Context, source io.ReaderAt, size int64, mediaType string, limits knowledgeOriginalLimits) error {
	if err := inspectKnowledgeOfficeDirectory(ctx, source, size, limits); err != nil {
		return err
	}
	archive, err := zip.NewReader(source, size)
	if err != nil {
		return errOriginalType
	}
	files := make(map[string]*zip.File, len(archive.File))
	var expanded uint64
	for _, file := range archive.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		key := strings.TrimSuffix(file.Name, "/")
		if !isKnowledgeOfficePath(file.Name) || files[key] != nil || file.Flags&(1|64) != 0 ||
			(file.Method != zip.Store && file.Method != zip.Deflate) ||
			(file.Mode()&os.ModeType != 0 && !file.FileInfo().IsDir()) ||
			file.CompressedSize64 > uint64(size) || file.UncompressedSize64 > limits.officeExpandedBytes-expanded {
			return errOriginalType
		}
		if file.UncompressedSize64 > 0 && (file.CompressedSize64 == 0 || file.UncompressedSize64/file.CompressedSize64 > limits.officeRatio ||
			(file.UncompressedSize64/file.CompressedSize64 == limits.officeRatio && file.UncompressedSize64%file.CompressedSize64 != 0)) {
			return errOriginalType
		}
		if strings.EqualFold(path.Base(file.Name), "vbaProject.bin") || strings.EqualFold(path.Base(file.Name), "vbaData.xml") {
			return errOriginalType
		}
		files[key] = file
		expanded += file.UncompressedSize64
	}
	mainPart, rootName := "word/document.xml", xml.Name{Space: "http://schemas.openxmlformats.org/wordprocessingml/2006/main", Local: "document"}
	mainType := "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"
	if mediaType == "application/vnd.openxmlformats-officedocument.presentationml.presentation" {
		mainPart = "ppt/presentation.xml"
		rootName = xml.Name{Space: "http://schemas.openxmlformats.org/presentationml/2006/main", Local: "presentation"}
		mainType = "application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"
	}
	contentTypeFound, relationshipFound := false, false
	relationshipIDs := map[string]bool{}
	var totalXML int64
	checks := []struct {
		name    string
		root    xml.Name
		inspect func(xml.StartElement) error
	}{
		{"[Content_Types].xml", xml.Name{Space: "http://schemas.openxmlformats.org/package/2006/content-types", Local: "Types"}, func(element xml.StartElement) error {
			if element.Name.Local == "Override" && element.Name.Space == "http://schemas.openxmlformats.org/package/2006/content-types" {
				partName, contentType := "", ""
				for _, attr := range element.Attr {
					if attr.Name.Space != "" {
						continue
					}
					if attr.Name.Local == "PartName" {
						partName = attr.Value
					}
					if attr.Name.Local == "ContentType" {
						contentType = attr.Value
					}
				}
				if partName == "/"+mainPart {
					if contentTypeFound || contentType != mainType {
						return errOriginalType
					}
					contentTypeFound = true
				}
			}
			return nil
		}},
		{"_rels/.rels", xml.Name{Space: "http://schemas.openxmlformats.org/package/2006/relationships", Local: "Relationships"}, func(element xml.StartElement) error {
			if element.Name.Local == "Relationship" && element.Name.Space == "http://schemas.openxmlformats.org/package/2006/relationships" {
				relType, target, mode, id := "", "", "", ""
				for _, attr := range element.Attr {
					if attr.Name.Space != "" {
						continue
					}
					if attr.Name.Local == "Id" {
						id = attr.Value
					}
					if attr.Name.Local == "Type" {
						relType = attr.Value
					}
					if attr.Name.Local == "Target" {
						target = attr.Value
					}
					if attr.Name.Local == "TargetMode" {
						mode = attr.Value
					}
				}
				if id == "" || relationshipIDs[id] || (mode != "" && mode != "Internal" && mode != "External") {
					return errOriginalType
				}
				relationshipIDs[id] = true
				if relType == "http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" {
					if relationshipFound || strings.TrimPrefix(target, "/") != mainPart || mode == "External" {
						return errOriginalType
					}
					relationshipFound = true
				}
			}
			return nil
		}},
		{mainPart, rootName, nil},
	}
	for _, check := range checks {
		file := files[check.name]
		if file == nil || file.FileInfo().IsDir() || file.UncompressedSize64 > uint64(limits.officeXMLBytes) ||
			file.UncompressedSize64 > uint64(limits.officeAllXMLBytes-totalXML) {
			return errOriginalType
		}
		read, err := inspectKnowledgeOfficeXML(ctx, file, check.root, check.inspect, min(limits.officeXMLBytes, limits.officeAllXMLBytes-totalXML), limits.officeXMLDepth)
		if err != nil {
			return err
		}
		totalXML += read
	}
	if !contentTypeFound || !relationshipFound {
		return errOriginalType
	}
	return nil
}

func inspectKnowledgeOfficeXML(ctx context.Context, file *zip.File, root xml.Name, inspect func(xml.StartElement) error, limit int64, depthLimit int) (int64, error) {
	body, err := file.Open()
	if err != nil {
		return 0, errOriginalType
	}
	defer body.Close()
	limited := &io.LimitedReader{R: &knowledgeContextReader{ctx: ctx, reader: body}, N: limit + 1}
	decoder := xml.NewDecoder(limited)
	depth, roots := 0, 0
	for {
		token, err := decoder.Token()
		read := limit + 1 - limited.N
		if read > limit {
			return read, errOriginalType
		}
		if errors.Is(err, io.EOF) {
			if roots != 1 || depth != 0 {
				return read, errOriginalType
			}
			return read, nil
		}
		if err != nil {
			if ctx.Err() != nil {
				return read, ctx.Err()
			}
			return read, errOriginalType
		}
		switch element := token.(type) {
		case xml.StartElement:
			if depth == 0 {
				roots++
				if roots != 1 || element.Name != root {
					return read, errOriginalType
				}
			}
			depth++
			if depth > depthLimit {
				return read, errOriginalType
			}
			attrs := map[xml.Name]bool{}
			for _, attr := range element.Attr {
				if attrs[attr.Name] {
					return read, errOriginalType
				}
				attrs[attr.Name] = true
			}
			if inspect != nil {
				if err := inspect(element); err != nil {
					return read, err
				}
			}
		case xml.EndElement:
			depth--
		case xml.Directive:
			return read, errOriginalType
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(element)) != "" {
				return read, errOriginalType
			}
		}
	}
}
