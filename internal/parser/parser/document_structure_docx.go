// Copyright 2026 LFen's Second Brain contributors.
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"encoding/xml"
	"strconv"
	"strings"

	"ragflow/internal/entity"
)

func (o *documentOffice) buildDOCX() error {
	main, err := o.tree("word/document.xml", xml.Name{Space: officeWordNS, Local: "document"})
	if err != nil {
		return err
	}
	rels, err := o.relationships(main.part)
	if err != nil {
		return err
	}
	for _, relation := range rels {
		if relation.kind != officeDocumentRelationshipsNS+"/styles" && relation.kind != officeDocumentRelationshipsNS+"/numbering" {
			continue
		}
		if relation.external {
			return ErrOfficePackage
		}
		local := "styles"
		if strings.HasSuffix(relation.kind, "/numbering") {
			local = "numbering"
		}
		tree, err := o.tree(relation.target, xml.Name{Space: officeWordNS, Local: local})
		if err != nil {
			return err
		}
		for _, child := range tree.children {
			switch {
			case child.is(officeWordNS, "style"):
				id := child.attr(officeWordNS, "styleId")
				if id == "" || o.styles[id] != nil {
					return ErrOfficePackage
				}
				o.styles[id] = child
			case child.is(officeWordNS, "num"):
				id := child.attr(officeWordNS, "numId")
				if id == "" || o.numbering[id] != nil {
					return ErrOfficePackage
				}
				o.numbering[id] = child
			case child.is(officeWordNS, "abstractNum"):
				id := child.attr(officeWordNS, "abstractNumId")
				if id == "" || o.abstracts[id] != nil {
					return ErrOfficePackage
				}
				o.abstracts[id] = child
			}
		}
	}
	body := main.child(officeWordNS, "body")
	if body == nil {
		return documentFailure("source_structure_unavailable")
	}
	section, err := o.b.add(o.b.root, "section", body.locator(), officePosition(), entity.KnowledgeDocumentEmptyAttributes{})
	if err != nil {
		return err
	}
	return o.docxBlocks(section, body.children, rels)
}

func (o *documentOffice) docxOutline(paragraph *officeSourceElement) (*int64, error) {
	properties := paragraph.child(officeWordNS, "pPr")
	var level *int64
	resolved := false
	readOutline := func(p *officeSourceElement) error {
		if p == nil {
			return nil
		}
		value, err := officeInteger(p.child(officeWordNS, "outlineLvl"), officeWordNS, "val", 0, 9)
		if err != nil {
			return err
		}
		if !resolved && value != nil {
			resolved = true
			if *value < 9 {
				level = documentInt(*value + 1)
			}
		}
		return nil
	}
	if err := readOutline(properties); err != nil {
		return nil, err
	}
	styleID := ""
	if properties != nil {
		if style := properties.child(officeWordNS, "pStyle"); style != nil {
			styleID = style.attr(officeWordNS, "val")
		}
	}
	seen := map[string]bool{}
	for depth := 0; styleID != ""; depth++ {
		if depth >= 32 || seen[styleID] {
			return nil, documentFailure("invalid_source_structure")
		}
		seen[styleID] = true
		style := o.styles[styleID]
		if style == nil {
			break
		}
		if err := readOutline(style.child(officeWordNS, "pPr")); err != nil {
			return nil, err
		}
		styleID = ""
		if based := style.child(officeWordNS, "basedOn"); based != nil {
			styleID = based.attr(officeWordNS, "val")
		}
	}
	return level, nil
}

func officeEnabled(element *officeSourceElement) bool {
	if element == nil {
		return false
	}
	value := element.attr(officeWordNS, "val")
	return value != "0" && value != "false" && value != "off"
}

func docxUnsupported(paragraph *officeSourceElement) string {
	var prohibited func(*officeSourceElement) string
	prohibited = func(element *officeSourceElement) string {
		if element.start.Name.Space == officeMarkupNS && element.start.Name.Local == "AlternateContent" {
			return "AlternateContent"
		}
		if element.start.Name.Space == officeWordNS {
			switch element.start.Name.Local {
			case "ins", "del", "moveFrom", "moveTo", "fldSimple", "fldChar", "instrText", "altChunk":
				return element.start.Name.Local
			case "vanish", "webHidden":
				if officeEnabled(element) {
					return element.start.Name.Local
				}
			}
		}
		for _, child := range element.children {
			if found := prohibited(child); found != "" {
				return found
			}
		}
		return ""
	}
	if found := prohibited(paragraph); found != "" {
		return found
	}
	var content func(*officeSourceElement) string
	content = func(element *officeSourceElement) string {
		if element.start.Name.Space != officeWordNS {
			return element.start.Name.Local
		}
		switch element.start.Name.Local {
		case "pPr", "rPr", "t", "tab", "br", "cr", "bookmarkStart", "bookmarkEnd", "proofErr", "commentRangeStart", "commentRangeEnd":
			return ""
		case "drawing":
			if len(officeDescendants(element, officeDrawingNS, "blip")) == 0 {
				return "drawing"
			}
			return ""
		case "p", "r", "hyperlink":
			for _, child := range element.children {
				if found := content(child); found != "" {
					return found
				}
			}
			return ""
		default:
			return element.start.Name.Local
		}
	}
	return content(paragraph)
}

func (o *documentOffice) docxInline(node *documentStructureNode, element *officeSourceElement, rels map[string]officeSourceRelationship) error {
	for _, child := range element.children {
		if err := o.b.ctx.Err(); err != nil {
			return err
		}
		switch {
		case child.is(officeWordNS, "t"):
			node.ownText += child.text
		case child.is(officeWordNS, "tab"):
			node.ownText += "\t"
		case child.is(officeWordNS, "br"), child.is(officeWordNS, "cr"):
			node.ownText += "\n"
		case child.is(officeWordNS, "drawing"):
			if err := o.media(node, child, officePosition(), rels, "inline", int64(len(node.ownText))); err != nil {
				return err
			}
		case child.is(officeWordNS, "r"), child.is(officeWordNS, "hyperlink"):
			if err := o.docxInline(node, child, rels); err != nil {
				return err
			}
		}
	}
	return nil
}

type documentListFrame struct {
	node, lastItem *documentStructureNode
	level          *int64
	id             string
	itemCount      int64
}

func (o *documentOffice) docxListAttributes(paragraph *officeSourceElement) (*entity.KnowledgeDocumentListAttributes, error) {
	properties := paragraph.child(officeWordNS, "pPr")
	if properties == nil {
		return nil, nil
	}
	numPr := properties.child(officeWordNS, "numPr")
	if numPr == nil {
		return nil, nil
	}
	attributes := &entity.KnowledgeDocumentListAttributes{}
	level, err := officeInteger(numPr.child(officeWordNS, "ilvl"), officeWordNS, "val", 0, 8)
	if err != nil {
		return nil, err
	}
	attributes.SourceLevel = level
	if id := numPr.child(officeWordNS, "numId"); id != nil {
		value := id.attr(officeWordNS, "val")
		if value != "" {
			if len(value) > 64 {
				return nil, ErrOfficePackage
			}
			if _, err := strconv.ParseUint(value, 10, 31); err != nil {
				return nil, ErrOfficePackage
			}
			attributes.NumberingID = documentString(value)
		}
	}
	if attributes.NumberingID == nil || level == nil {
		return attributes, nil
	}
	num := o.numbering[*attributes.NumberingID]
	if num == nil {
		return attributes, nil
	}
	var definition *officeSourceElement
	if abstract := num.child(officeWordNS, "abstractNumId"); abstract != nil {
		if source := o.abstracts[abstract.attr(officeWordNS, "val")]; source != nil {
			for _, child := range source.children {
				if child.is(officeWordNS, "lvl") && child.attr(officeWordNS, "ilvl") == strconv.FormatInt(*level, 10) {
					definition = child
				}
			}
		}
	}
	var override *officeSourceElement
	for _, child := range num.children {
		if child.is(officeWordNS, "lvlOverride") && child.attr(officeWordNS, "ilvl") == strconv.FormatInt(*level, 10) {
			override = child
			if candidate := child.child(officeWordNS, "lvl"); candidate != nil {
				definition = candidate
			}
		}
	}
	if definition != nil {
		attributes.StartNumber, err = officeInteger(definition.child(officeWordNS, "start"), officeWordNS, "val", 0, 2147483647)
		if err != nil {
			return nil, err
		}
		if format := definition.child(officeWordNS, "numFmt"); format != nil {
			value := format.attr(officeWordNS, "val")
			switch value {
			case "bullet", "decimal", "upperLetter", "lowerLetter", "upperRoman", "lowerRoman":
				attributes.NumberFormat = documentString(value)
				ordered := value != "bullet"
				attributes.Ordered = &ordered
			}
		}
	}
	if override != nil {
		if start := override.child(officeWordNS, "startOverride"); start != nil {
			attributes.StartNumber, err = officeInteger(start, officeWordNS, "val", 0, 2147483647)
			if err != nil {
				return nil, err
			}
		}
	}
	return attributes, nil
}

func (o *documentOffice) docxBlocks(parent *documentStructureNode, elements []*officeSourceElement, rels map[string]officeSourceRelationship) error {
	headings := []*documentStructureNode{parent}
	headingLevels := []int64{0}
	lists := []*documentListFrame{}
	for _, source := range elements {
		if err := o.b.ctx.Err(); err != nil {
			return err
		}
		current := headings[len(headings)-1]
		switch {
		case source.is(officeWordNS, "sectPr"), source.is(officeWordNS, "tcPr"), source.is(officeWordNS, "trPr"), source.is(officeWordNS, "tblPr"), source.is(officeWordNS, "tblGrid"):
			continue
		case source.is(officeWordNS, "sdt"):
			lists = nil
			content := source.child(officeWordNS, "sdtContent")
			if content == nil {
				return ErrOfficePackage
			}
			if err := o.docxBlocks(current, content.children, rels); err != nil {
				return err
			}
		case source.is(officeWordNS, "tbl"):
			lists = nil
			if err := o.table(current, source, officePosition(), officeWordNS, rels); err != nil {
				return err
			}
		case source.is(officeWordNS, "p"):
			if unsupported := docxUnsupported(source); unsupported != "" {
				lists = nil
				if err := o.b.unsupported(current, source.locator(), officePosition(), unsupported); err != nil {
					return err
				}
				continue
			}
			level, err := o.docxOutline(source)
			if err != nil {
				return err
			}
			attributes, err := o.docxListAttributes(source)
			if err != nil {
				return err
			}
			if level != nil {
				lists = nil
				for len(headings) > 1 && headingLevels[len(headingLevels)-1] >= *level {
					headings = headings[:len(headings)-1]
					headingLevels = headingLevels[:len(headingLevels)-1]
				}
				current = headings[len(headings)-1]
				node, err := o.b.add(current, "heading", source.locator(), officePosition(), entity.KnowledgeDocumentHeadingAttributes{Level: level})
				if err != nil {
					return err
				}
				node.own = true
				if err := o.docxInline(node, source, rels); err != nil {
					return err
				}
				headings = append(headings, node)
				headingLevels = append(headingLevels, *level)
				continue
			}
			if attributes != nil {
				id := ""
				if attributes.NumberingID != nil {
					id = *attributes.NumberingID
				}
				for len(lists) > 0 && (lists[len(lists)-1].id != id || attributes.SourceLevel == nil || lists[len(lists)-1].level == nil || *lists[len(lists)-1].level > *attributes.SourceLevel) {
					lists = lists[:len(lists)-1]
				}
				var frame *documentListFrame
				if len(lists) > 0 && *lists[len(lists)-1].level == *attributes.SourceLevel {
					frame = lists[len(lists)-1]
				} else {
					owner := current
					if len(lists) > 0 && lists[len(lists)-1].lastItem != nil {
						owner = lists[len(lists)-1].lastItem
					}
					properties := source.child(officeWordNS, "pPr")
					numberProperties := properties.child(officeWordNS, "numPr")
					listSource := numberProperties
					if idElement := numberProperties.child(officeWordNS, "numId"); idElement != nil {
						listSource = idElement
					} else if levelElement := numberProperties.child(officeWordNS, "ilvl"); levelElement != nil {
						listSource = levelElement
					}
					list, err := o.b.add(owner, "list", listSource.locator(), officePosition(), *attributes)
					if err != nil {
						return err
					}
					frame = &documentListFrame{node: list, level: attributes.SourceLevel, id: id}
					lists = append(lists, frame)
				}
				item, err := o.b.add(frame.node, "list_item", source.child(officeWordNS, "pPr").locator(), officePosition(), entity.KnowledgeDocumentListItemAttributes{ItemIndex: frame.itemCount})
				if err != nil {
					return err
				}
				frame.itemCount++
				frame.lastItem = item
				current = item
			} else {
				lists = nil
			}
			node, err := o.b.add(current, "paragraph", source.locator(), officePosition(), entity.KnowledgeDocumentEmptyAttributes{})
			if err != nil {
				return err
			}
			node.own = true
			if err := o.docxInline(node, source, rels); err != nil {
				return err
			}
		default:
			lists = nil
			if err := o.b.unsupported(current, source.locator(), officePosition(), source.start.Name.Local); err != nil {
				return err
			}
		}
	}
	return nil
}
