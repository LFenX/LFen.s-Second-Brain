// Copyright 2026 LFen's Second Brain contributors.
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"path"
	"strconv"
	"strings"

	"ragflow/internal/entity"
)

const (
	officeWordNS                  = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	officeDrawingNS               = "http://schemas.openxmlformats.org/drawingml/2006/main"
	officePresentationNS          = "http://schemas.openxmlformats.org/presentationml/2006/main"
	officeRelationshipsNS         = "http://schemas.openxmlformats.org/package/2006/relationships"
	officeDocumentRelationshipsNS = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	officeMarkupNS                = "http://schemas.openxmlformats.org/markup-compatibility/2006"
)

type officeSourceElement struct {
	start    xml.StartElement
	text     string
	children []*officeSourceElement
	parent   *officeSourceElement
	index    int64
	part     string
}

func (e *officeSourceElement) attr(namespace, name string) string {
	return officeAttribute(e.start, namespace, name)
}
func (e *officeSourceElement) is(namespace, name string) bool {
	return e.start.Name.Space == namespace && e.start.Name.Local == name
}
func (e *officeSourceElement) child(namespace, name string) *officeSourceElement {
	for _, child := range e.children {
		if child.is(namespace, name) {
			return child
		}
	}
	return nil
}
func (e *officeSourceElement) locator() entity.KnowledgeDocumentLocator {
	segments := []entity.KnowledgeDocumentOfficeSegment{}
	for current := e; current != nil; current = current.parent {
		segments = append(segments, entity.KnowledgeDocumentOfficeSegment{Namespace: current.start.Name.Space, Name: current.start.Name.Local, Index: current.index})
	}
	for i, j := 0, len(segments)-1; i < j; i, j = i+1, j-1 {
		segments[i], segments[j] = segments[j], segments[i]
	}
	return entity.KnowledgeDocumentLocator{Scheme: "office_package_v1", Part: e.part, Segments: segments}
}
func officeDescendants(element *officeSourceElement, namespace, name string) []*officeSourceElement {
	result := []*officeSourceElement{}
	var visit func(*officeSourceElement)
	visit = func(current *officeSourceElement) {
		if current.is(namespace, name) {
			result = append(result, current)
		}
		for _, child := range current.children {
			visit(child)
		}
	}
	visit(element)
	return result
}

type documentOffice struct {
	b                 *documentStructureBuilder
	pkg               *OfficePackage
	styles            map[string]*officeSourceElement
	numbering         map[string]*officeSourceElement
	abstracts         map[string]*officeSourceElement
	mediaTypes        map[string]string
	sourceTrees       map[string]*officeSourceElement
	intermediateBytes int
}

func (b *documentStructureBuilder) buildOffice(format string, data []byte) error {
	limits := OfficePackageLimits{Entries: 4096, ExpandedBytes: 16 << 20, Ratio: 100, XMLBytes: 4 << 20, AllXMLBytes: 16 << 20, DirectoryBytes: 2 << 20, XMLDepth: 64}
	pkg, err := OpenOfficePackage(b.ctx, bytes.NewReader(data), int64(len(data)), documentMediaType(format), limits)
	if err != nil {
		return err
	}
	o := &documentOffice{b: b, pkg: pkg, styles: map[string]*officeSourceElement{}, numbering: map[string]*officeSourceElement{}, abstracts: map[string]*officeSourceElement{}, mediaTypes: map[string]string{}, sourceTrees: map[string]*officeSourceElement{}}
	types, err := o.tree("[Content_Types].xml", xml.Name{Space: "http://schemas.openxmlformats.org/package/2006/content-types", Local: "Types"})
	if err != nil {
		return err
	}
	for _, element := range types.children {
		if element.start.Name.Local == "Default" {
			o.mediaTypes["*."+strings.ToLower(element.attr("", "Extension"))] = element.attr("", "ContentType")
		}
		if element.start.Name.Local == "Override" {
			o.mediaTypes[strings.TrimPrefix(element.attr("", "PartName"), "/")] = element.attr("", "ContentType")
		}
	}
	if format == "pptx" {
		return o.buildPPTX()
	}
	return o.buildDOCX()
}

func (o *documentOffice) tree(part string, root xml.Name) (*officeSourceElement, error) {
	if cached := o.sourceTrees[part]; cached != nil {
		if cached.start.Name != root {
			return nil, ErrOfficePackage
		}
		return cached, nil
	}
	data, err := o.pkg.XML(part, root)
	if err != nil {
		return nil, err
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var tree, current *officeSourceElement
	counts := map[*officeSourceElement]map[xml.Name]int64{}
	for {
		if err := o.b.ctx.Err(); err != nil {
			return nil, err
		}
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, ErrOfficePackage
		}
		switch value := token.(type) {
		case xml.StartElement:
			index := int64(0)
			if current != nil {
				if counts[current] == nil {
					counts[current] = map[xml.Name]int64{}
				}
				index = counts[current][value.Name]
				counts[current][value.Name]++
			}
			// Account a bounded flat source-token representation before allocating
			// the node. This is the private intermediate-pack budget, not a new
			// public node or ZIP-entry limit.
			encoded, _ := json.Marshal(struct {
				Name   xml.Name
				Attr   []xml.Attr
				Parent bool
			}{value.Name, value.Attr, current != nil})
			o.intermediateBytes += len(encoded) + 16
			if o.intermediateBytes > 16<<20 {
				return nil, documentFailure("budget_exceeded")
			}
			node := &officeSourceElement{start: value, parent: current, index: index, part: part}
			if current != nil {
				current.children = append(current.children, node)
			} else {
				tree = node
			}
			current = node
		case xml.EndElement:
			current = current.parent
		case xml.CharData:
			if current != nil {
				encoded, _ := json.Marshal(string(value))
				o.intermediateBytes += len(encoded)
				if o.intermediateBytes > 16<<20 {
					return nil, documentFailure("budget_exceeded")
				}
				current.text += string(value)
			}
		}
	}
	if tree == nil {
		return nil, ErrOfficePackage
	}
	o.sourceTrees[part] = tree
	return tree, nil
}

type officeSourceRelationship struct {
	kind, target string
	external     bool
}

func (o *documentOffice) relationships(part string) (map[string]officeSourceRelationship, error) {
	name := path.Join(path.Dir(part), "_rels", path.Base(part)+".rels")
	result := map[string]officeSourceRelationship{}
	if o.pkg.files[name] == nil {
		return result, nil
	}
	root, err := o.tree(name, xml.Name{Space: officeRelationshipsNS, Local: "Relationships"})
	if err != nil {
		return nil, err
	}
	for _, entry := range root.children {
		if !entry.is(officeRelationshipsNS, "Relationship") {
			return nil, ErrOfficePackage
		}
		id, mode, target := entry.attr("", "Id"), entry.attr("", "TargetMode"), entry.attr("", "Target")
		if id == "" || len(id) > 256 || target == "" {
			return nil, ErrOfficePackage
		}
		if _, duplicate := result[id]; duplicate {
			return nil, ErrOfficePackage
		}
		if mode != "" && mode != "Internal" && mode != "External" {
			return nil, ErrOfficePackage
		}
		relation := officeSourceRelationship{kind: entry.attr("", "Type"), external: mode == "External"}
		if !relation.external {
			if strings.ContainsAny(target, "\\:\x00?#") {
				return nil, ErrOfficePackage
			}
			if strings.HasPrefix(target, "/") {
				relation.target = strings.TrimPrefix(target, "/")
			} else {
				relation.target = path.Clean(path.Join(path.Dir(part), target))
			}
			if !isOfficePath(relation.target) || o.pkg.files[relation.target] == nil {
				return nil, ErrOfficePackage
			}
		}
		result[id] = relation
	}
	return result, nil
}

func officeInteger(element *officeSourceElement, namespace, name string, low, high int64) (*int64, error) {
	if element == nil {
		return nil, nil
	}
	value := element.attr(namespace, name)
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < low || parsed > high {
		return nil, documentFailure("invalid_source_structure")
	}
	return documentInt(parsed), nil
}

func (o *documentOffice) media(parent *documentStructureNode, source *officeSourceElement, position entity.KnowledgeDocumentOriginalPosition, rels map[string]officeSourceRelationship, placement string, offset int64) error {
	attributes := entity.KnowledgeDocumentMediaAttributes{Placement: placement, Origin: "unknown", OCRStatus: "not_requested"}
	for _, blip := range officeDescendants(source, officeDrawingNS, "blip") {
		id := blip.attr(officeDocumentRelationshipsNS, "embed")
		if id == "" {
			id = blip.attr(officeDocumentRelationshipsNS, "link")
		}
		rel, found := rels[id]
		if !found {
			return documentFailure("invalid_source_structure")
		}
		if rel.kind != officeDocumentRelationshipsNS+"/image" {
			return documentFailure("invalid_source_structure")
		}
		if rel.external {
			attributes.Origin = "external_reference"
		} else {
			attributes.Origin = "embedded"
			file := o.pkg.files[rel.target]
			if file == nil {
				return ErrOfficePackage
			}
			attributes.ByteLength = documentInt(int64(file.UncompressedSize64))
			mediaType := o.mediaTypes[rel.target]
			if mediaType == "" {
				mediaType = o.mediaTypes["*."+strings.ToLower(strings.TrimPrefix(path.Ext(rel.target), "."))]
			}
			if mediaType != "" {
				attributes.MediaType = documentString(mediaType)
			}
		}
		break
	}
	if placement == "inline" {
		attributes.InlineAnchorByte = documentInt(0)
	}
	node, err := o.b.add(parent, "media", source.locator(), position, attributes)
	if err != nil {
		return err
	}
	node.inline = placement == "inline"
	node.inlineOffset = offset
	node.payload.TextStatus = "unsupported"
	node.payload.TextReason = documentString("media_not_ocr")
	o.b.diagnostic("media_not_ocr", &node.payload.Locator)
	return nil
}

func officePosition() entity.KnowledgeDocumentOriginalPosition {
	return entity.KnowledgeDocumentOriginalPosition{Kind: "document_only"}
}
