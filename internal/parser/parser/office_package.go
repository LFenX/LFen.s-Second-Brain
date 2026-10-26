// Copyright 2026 LFen's Second Brain contributors.
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"archive/zip"
	"bytes"
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

var ErrOfficePackage = errors.New("invalid Office package")
var ErrOfficePackageBudget = errors.New("Office package budget exceeded")

// OfficePackageLimits is supplied by the owning admission policy. The .5
// original admission and document production deliberately use different values.
type OfficePackageLimits struct {
	Entries        int
	ExpandedBytes  uint64
	Ratio          uint64
	XMLBytes       int64
	AllXMLBytes    int64
	DirectoryBytes int64
	XMLDepth       int
}

type OfficePackage struct {
	ctx      context.Context
	files    map[string]*zip.File
	limits   OfficePackageLimits
	xmlBytes int64
	cache    map[string][]byte
}

// OpenOfficePackage is the owning extraction of the original .5 ZIP/XML
// validation. Admission delegates here instead of retaining another ZIP reader.
func OpenOfficePackage(ctx context.Context, source io.ReaderAt, size int64, mediaType string, limits OfficePackageLimits) (*OfficePackage, error) {
	if limits.Entries < 1 || limits.Entries >= 65535 || limits.ExpandedBytes < 1 || limits.Ratio < 1 || limits.XMLBytes < 1 || limits.AllXMLBytes < limits.XMLBytes || limits.DirectoryBytes < 1 || limits.XMLDepth < 1 {
		return nil, ErrOfficePackage
	}
	if err := inspectOfficeDirectory(ctx, source, size, limits); err != nil {
		return nil, err
	}
	archive, err := zip.NewReader(source, size)
	if err != nil {
		return nil, ErrOfficePackage
	}
	p := &OfficePackage{ctx: ctx, files: make(map[string]*zip.File, len(archive.File)), limits: limits, cache: map[string][]byte{}}
	var expanded uint64
	for _, file := range archive.File {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if file.UncompressedSize64 > limits.ExpandedBytes-expanded {
			return nil, ErrOfficePackageBudget
		}
		key := strings.TrimSuffix(file.Name, "/")
		if !isOfficePath(file.Name) || p.files[key] != nil || file.Flags&(1|64) != 0 || (file.Method != zip.Store && file.Method != zip.Deflate) ||
			(file.Mode()&os.ModeType != 0 && !file.FileInfo().IsDir()) || file.CompressedSize64 > uint64(size) || file.UncompressedSize64 > limits.ExpandedBytes-expanded {
			return nil, ErrOfficePackage
		}
		if file.UncompressedSize64 > 0 && (file.CompressedSize64 == 0 || file.UncompressedSize64/file.CompressedSize64 > limits.Ratio ||
			(file.UncompressedSize64/file.CompressedSize64 == limits.Ratio && file.UncompressedSize64%file.CompressedSize64 != 0)) {
			return nil, ErrOfficePackage
		}
		if strings.EqualFold(path.Base(file.Name), "vbaProject.bin") || strings.EqualFold(path.Base(file.Name), "vbaData.xml") {
			return nil, ErrOfficePackage
		}
		p.files[key] = file
		expanded += file.UncompressedSize64
	}
	mainPart, mainRoot, mainType := "word/document.xml", xml.Name{Space: officeWordNS, Local: "document"}, "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"
	if mediaType == "application/vnd.openxmlformats-officedocument.presentationml.presentation" {
		mainPart, mainRoot, mainType = "ppt/presentation.xml", xml.Name{Space: officePresentationNS, Local: "presentation"}, "application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"
	}
	contentTypeFound, relationshipFound := false, false
	ids := map[string]bool{}
	checks := []struct {
		name    string
		root    xml.Name
		inspect func(xml.StartElement) error
	}{
		{"[Content_Types].xml", xml.Name{Space: "http://schemas.openxmlformats.org/package/2006/content-types", Local: "Types"}, func(element xml.StartElement) error {
			if element.Name == (xml.Name{Space: "http://schemas.openxmlformats.org/package/2006/content-types", Local: "Override"}) {
				part, kind := officeAttribute(element, "", "PartName"), officeAttribute(element, "", "ContentType")
				if part == "/"+mainPart {
					if contentTypeFound || kind != mainType {
						return ErrOfficePackage
					}
					contentTypeFound = true
				}
			}
			return nil
		}},
		{"_rels/.rels", xml.Name{Space: officeRelationshipsNS, Local: "Relationships"}, func(element xml.StartElement) error {
			if element.Name == (xml.Name{Space: officeRelationshipsNS, Local: "Relationship"}) {
				id, mode := officeAttribute(element, "", "Id"), officeAttribute(element, "", "TargetMode")
				if id == "" || ids[id] || (mode != "" && mode != "Internal" && mode != "External") {
					return ErrOfficePackage
				}
				ids[id] = true
				if officeAttribute(element, "", "Type") == officeDocumentRelationshipsNS+"/officeDocument" {
					if relationshipFound || strings.TrimPrefix(officeAttribute(element, "", "Target"), "/") != mainPart || mode == "External" {
						return ErrOfficePackage
					}
					relationshipFound = true
				}
			}
			return nil
		}},
		{mainPart, mainRoot, nil},
	}
	for _, check := range checks {
		data, err := p.XML(check.name, check.root)
		if err != nil {
			return nil, err
		}
		if check.inspect != nil {
			if err := scanOfficeXML(ctx, data, check.root, limits.XMLDepth, check.inspect); err != nil {
				return nil, err
			}
		}
	}
	if !contentTypeFound || !relationshipFound {
		return nil, ErrOfficePackage
	}
	return p, nil
}

// XML validates a part once, counts actual bytes once and retains a bounded
// cache for the source decoder. It never interprets external relationships.
func (p *OfficePackage) XML(name string, root xml.Name) ([]byte, error) {
	if cached, ok := p.cache[name]; ok {
		if err := scanOfficeXML(p.ctx, cached, root, p.limits.XMLDepth, nil); err != nil {
			return nil, err
		}
		return cached, nil
	}
	file := p.files[name]
	remaining := p.limits.AllXMLBytes - p.xmlBytes
	if file == nil || file.FileInfo().IsDir() {
		return nil, ErrOfficePackage
	}
	if remaining < 0 || file.UncompressedSize64 > uint64(min(p.limits.XMLBytes, remaining)) {
		return nil, ErrOfficePackageBudget
	}
	data, err := p.readPart(name, min(p.limits.XMLBytes, remaining))
	if err != nil {
		return nil, err
	}
	if err := scanOfficeXML(p.ctx, data, root, p.limits.XMLDepth, nil); err != nil {
		return nil, err
	}
	p.xmlBytes += int64(len(data))
	p.cache[name] = data
	return data, nil
}

func (p *OfficePackage) readPart(name string, limit int64) ([]byte, error) {
	file := p.files[name]
	if file == nil || file.FileInfo().IsDir() || limit < 0 || file.UncompressedSize64 > uint64(limit) {
		return nil, ErrOfficePackage
	}
	reader, err := file.Open()
	if err != nil {
		return nil, ErrOfficePackage
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(&officeContextReader{ctx: p.ctx, reader: reader}, limit+1))
	if err != nil {
		if p.ctx.Err() != nil {
			return nil, p.ctx.Err()
		}
		return nil, ErrOfficePackage
	}
	if int64(len(data)) > limit || uint64(len(data)) != file.UncompressedSize64 {
		return nil, ErrOfficePackage
	}
	return data, nil
}

type officeContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *officeContextReader) Read(data []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(data)
}

func officeAttribute(element xml.StartElement, namespace, name string) string {
	for _, attribute := range element.Attr {
		if attribute.Name.Space == namespace && attribute.Name.Local == name {
			return attribute.Value
		}
	}
	return ""
}

func scanOfficeXML(ctx context.Context, data []byte, root xml.Name, depthLimit int, inspect func(xml.StartElement) error) error {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	depth, roots := 0, 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			if roots != 1 || depth != 0 {
				return ErrOfficePackage
			}
			return nil
		}
		if err != nil {
			return ErrOfficePackage
		}
		switch element := token.(type) {
		case xml.StartElement:
			if depth == 0 {
				roots++
				if roots != 1 || element.Name != root {
					return ErrOfficePackage
				}
			}
			depth++
			if depth > depthLimit {
				return ErrOfficePackage
			}
			attrs := map[xml.Name]bool{}
			for _, attribute := range element.Attr {
				if attrs[attribute.Name] {
					return ErrOfficePackage
				}
				attrs[attribute.Name] = true
			}
			if inspect != nil {
				if err := inspect(element); err != nil {
					return err
				}
			}
		case xml.EndElement:
			depth--
		case xml.Directive:
			return ErrOfficePackage
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(element)) != "" {
				return ErrOfficePackage
			}
		}
	}
}

func inspectOfficeDirectory(ctx context.Context, source io.ReaderAt, size int64, limits OfficePackageLimits) error {
	if size < 22 {
		return ErrOfficePackage
	}
	var signature [4]byte
	if _, err := source.ReadAt(signature[:], 0); err != nil {
		return err
	}
	if binary.LittleEndian.Uint32(signature[:]) != 0x04034b50 {
		return ErrOfficePackage
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
		return ErrOfficePackage
	}
	record := tail[end:]
	entries := int(binary.LittleEndian.Uint16(record[10:]))
	directoryBytes := int64(binary.LittleEndian.Uint32(record[12:]))
	offset := int64(binary.LittleEndian.Uint32(record[16:]))
	endOffset := size - int64(len(tail)) + int64(end)
	if entries > limits.Entries || directoryBytes > limits.DirectoryBytes {
		return ErrOfficePackageBudget
	}
	if binary.LittleEndian.Uint16(record[4:]) != 0 || binary.LittleEndian.Uint16(record[6:]) != 0 || int(binary.LittleEndian.Uint16(record[8:])) != entries || entries < 1 || entries > limits.Entries || entries == 65535 || directoryBytes <= 0 || directoryBytes > limits.DirectoryBytes || offset+directoryBytes != endOffset {
		return ErrOfficePackage
	}
	position, count := offset, 0
	for position < endOffset {
		if err := ctx.Err(); err != nil {
			return err
		}
		var header [46]byte
		if endOffset-position < int64(len(header)) {
			return ErrOfficePackage
		}
		if _, err := source.ReadAt(header[:], position); err != nil {
			return err
		}
		if binary.LittleEndian.Uint32(header[:]) != 0x02014b50 || binary.LittleEndian.Uint16(header[34:]) != 0 || binary.LittleEndian.Uint32(header[20:]) == 0xffffffff || binary.LittleEndian.Uint32(header[24:]) == 0xffffffff || binary.LittleEndian.Uint32(header[42:]) == 0xffffffff {
			return ErrOfficePackage
		}
		length := int64(len(header)) + int64(binary.LittleEndian.Uint16(header[28:])) + int64(binary.LittleEndian.Uint16(header[30:])) + int64(binary.LittleEndian.Uint16(header[32:]))
		if length > endOffset-position || count >= limits.Entries {
			return ErrOfficePackage
		}
		position += length
		count++
	}
	if position != endOffset || count != entries {
		return ErrOfficePackage
	}
	return nil
}

func isOfficePath(name string) bool {
	clean := strings.TrimSuffix(name, "/")
	if clean == "" || len(name) > 512 || !utf8.ValidString(name) || strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\:") || strings.ContainsFunc(name, unicode.IsControl) || path.Clean(clean) != clean {
		return false
	}
	for _, part := range strings.Split(clean, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}
