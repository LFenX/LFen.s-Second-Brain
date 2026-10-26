// Copyright 2026 LFen's Second Brain contributors.
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"encoding/xml"

	"ragflow/internal/entity"
)

func (o *documentOffice) buildPPTX() error {
	main, err := o.tree("ppt/presentation.xml", xml.Name{Space: officePresentationNS, Local: "presentation"})
	if err != nil {
		return err
	}
	list := main.child(officePresentationNS, "sldIdLst")
	if list == nil {
		return documentFailure("source_structure_unavailable")
	}
	if len(list.children) < 1 {
		return documentFailure("source_structure_unavailable")
	}
	if len(list.children) > 128 {
		return documentFailure("budget_exceeded")
	}
	rels, err := o.relationships(main.part)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for index, entry := range list.children {
		if !entry.is(officePresentationNS, "sldId") {
			return ErrOfficePackage
		}
		id := entry.attr(officeDocumentRelationshipsNS, "id")
		relation, found := rels[id]
		if !found || seen[id] || relation.external || relation.kind != officeDocumentRelationshipsNS+"/slide" {
			return ErrOfficePackage
		}
		seen[id] = true
		source, err := o.tree(relation.target, xml.Name{Space: officePresentationNS, Local: "sld"})
		if err != nil {
			return err
		}
		show := source.attr("", "show")
		hidden := show == "0" || show == "false" || show == "off"
		if show != "" && show != "1" && show != "true" && show != "on" && !hidden {
			return ErrOfficePackage
		}
		position := entity.KnowledgeDocumentOriginalPosition{Kind: "pptx_slide", SlideNumber: int64(index + 1)}
		node, err := o.b.add(o.b.root, "slide", source.locator(), position, entity.KnowledgeDocumentSlideAttributes{SlideNumber: int64(index + 1), Hidden: hidden})
		if err != nil {
			return err
		}
		common := source.child(officePresentationNS, "cSld")
		if common == nil {
			return ErrOfficePackage
		}
		shapes := common.child(officePresentationNS, "spTree")
		if shapes == nil {
			return ErrOfficePackage
		}
		slideRels, err := o.relationships(source.part)
		if err != nil {
			return err
		}
		if err := o.pptxShapes(node, shapes.children, position, slideRels); err != nil {
			return err
		}
		if !documentHasOwnText(node) {
			node.payload.TextStatus = "unsupported_or_requires_ocr"
			node.payload.TextReason = documentString("no_extractable_text")
			o.b.diagnostic("no_extractable_text", &node.payload.Locator)
		}
	}
	o.b.result.SlideCount = documentInt(int64(len(list.children)))
	return nil
}

func documentHasOwnText(node *documentStructureNode) bool {
	if node.own && node.ownText != "" {
		return true
	}
	for _, child := range node.children {
		if documentHasOwnText(child) {
			return true
		}
	}
	return false
}

func (o *documentOffice) pptxShapes(parent *documentStructureNode, elements []*officeSourceElement, position entity.KnowledgeDocumentOriginalPosition, rels map[string]officeSourceRelationship) error {
	for _, source := range elements {
		if err := o.b.ctx.Err(); err != nil {
			return err
		}
		switch {
		case source.is(officePresentationNS, "nvGrpSpPr"), source.is(officePresentationNS, "grpSpPr"):
			continue
		case source.is(officePresentationNS, "grpSp"):
			node, err := o.b.add(parent, "text_box", source.locator(), position, entity.KnowledgeDocumentEmptyAttributes{})
			if err != nil {
				return err
			}
			if err := o.pptxShapes(node, source.children, position, rels); err != nil {
				return err
			}
		case source.is(officePresentationNS, "sp"):
			body := source.child(officePresentationNS, "txBody")
			if body == nil {
				if err := o.b.unsupported(parent, source.locator(), position, "sp"); err != nil {
					return err
				}
				continue
			}
			node, err := o.b.add(parent, "text_box", source.locator(), position, entity.KnowledgeDocumentEmptyAttributes{})
			if err != nil {
				return err
			}
			heading := false
			for _, ph := range officeDescendants(source, officePresentationNS, "ph") {
				value := ph.attr("", "type")
				if value == "title" || value == "ctrTitle" {
					heading = true
				}
			}
			if err := o.pptxText(node, body, position, rels, heading); err != nil {
				return err
			}
		case source.is(officePresentationNS, "graphicFrame"):
			graphic := source.child(officeDrawingNS, "graphic")
			var table *officeSourceElement
			if graphic != nil {
				if data := graphic.child(officeDrawingNS, "graphicData"); data != nil && data.attr("", "uri") == "http://schemas.openxmlformats.org/drawingml/2006/table" {
					table = data.child(officeDrawingNS, "tbl")
				}
			}
			if table == nil {
				if err := o.b.unsupported(parent, source.locator(), position, "graphicFrame"); err != nil {
					return err
				}
				continue
			}
			if err := o.table(parent, table, position, officeDrawingNS, rels); err != nil {
				return err
			}
		case source.is(officePresentationNS, "pic"):
			if len(officeDescendants(source, officeDrawingNS, "blip")) == 0 {
				if err := o.b.unsupported(parent, source.locator(), position, "pic"); err != nil {
					return err
				}
				continue
			}
			if err := o.media(parent, source, position, rels, "block", 0); err != nil {
				return err
			}
		default:
			if err := o.b.unsupported(parent, source.locator(), position, source.start.Name.Local); err != nil {
				return err
			}
		}
	}
	return nil
}

func pptxUnsupported(element *officeSourceElement) string {
	if element.start.Name.Space != officeDrawingNS {
		return element.start.Name.Local
	}
	switch element.start.Name.Local {
	case "pPr", "rPr", "endParaRPr", "t", "tab", "br":
		return ""
	case "p", "r":
		for _, child := range element.children {
			if found := pptxUnsupported(child); found != "" {
				return found
			}
		}
		return ""
	default:
		return element.start.Name.Local
	}
}

func pptxInline(element *officeSourceElement) string {
	text := ""
	for _, child := range element.children {
		switch {
		case child.is(officeDrawingNS, "t"):
			text += child.text
		case child.is(officeDrawingNS, "tab"):
			text += "\t"
		case child.is(officeDrawingNS, "br"):
			text += "\n"
		case child.is(officeDrawingNS, "r"):
			text += pptxInline(child)
		}
	}
	return text
}

func (o *documentOffice) pptxText(parent *documentStructureNode, body *officeSourceElement, position entity.KnowledgeDocumentOriginalPosition, rels map[string]officeSourceRelationship, heading bool) error {
	var lists []*documentListFrame
	for _, source := range body.children {
		if source.is(officeDrawingNS, "bodyPr") || source.is(officeDrawingNS, "lstStyle") {
			continue
		}
		if !source.is(officeDrawingNS, "p") {
			lists = nil
			if err := o.b.unsupported(parent, source.locator(), position, source.start.Name.Local); err != nil {
				return err
			}
			continue
		}
		if nativeType := pptxUnsupported(source); nativeType != "" {
			lists = nil
			if err := o.b.unsupported(parent, source.locator(), position, nativeType); err != nil {
				return err
			}
			continue
		}
		owner := parent
		properties := source.child(officeDrawingNS, "pPr")
		var bullet, number *officeSourceElement
		if properties != nil {
			bullet = properties.child(officeDrawingNS, "buChar")
			number = properties.child(officeDrawingNS, "buAutoNum")
		}
		if bullet != nil && number != nil {
			return ErrOfficePackage
		}
		if bullet != nil || number != nil {
			attrs := entity.KnowledgeDocumentListAttributes{}
			level, err := officeInteger(properties, "", "lvl", 0, 8)
			if err != nil {
				return err
			}
			attrs.SourceLevel = level
			ordered := number != nil
			attrs.Ordered = &ordered
			key := "bullet:"
			if bullet != nil {
				attrs.NumberFormat = documentString("bullet")
				key += bullet.attr("", "char")
			}
			if number != nil {
				format := number.attr("", "type")
				if len(format) > 64 {
					return ErrOfficePackage
				}
				if format != "" {
					attrs.NumberFormat = documentString(format)
				}
				attrs.StartNumber, err = officeInteger(number, "", "startAt", 0, 2147483647)
				if err != nil {
					return err
				}
				key = "number:" + format
			}
			if number != nil {
				key += ":" + number.attr("", "startAt")
			}
			listSource := bullet
			if listSource == nil {
				listSource = number
			}
			for len(lists) > 0 {
				top := lists[len(lists)-1]
				if top.id != key || (top.level == nil) != (level == nil) {
					lists = nil
					break
				}
				if top.level != nil && *top.level > *level {
					lists = lists[:len(lists)-1]
					continue
				}
				break
			}
			var frame *documentListFrame
			if len(lists) > 0 && (level == nil || *lists[len(lists)-1].level == *level) {
				frame = lists[len(lists)-1]
			} else {
				listOwner := parent
				if len(lists) > 0 && lists[len(lists)-1].lastItem != nil {
					listOwner = lists[len(lists)-1].lastItem
				}
				list, err := o.b.add(listOwner, "list", listSource.locator(), position, attrs)
				if err != nil {
					return err
				}
				frame = &documentListFrame{node: list, level: level, id: key}
				lists = append(lists, frame)
			}
			item, err := o.b.add(frame.node, "list_item", properties.locator(), position, entity.KnowledgeDocumentListItemAttributes{ItemIndex: frame.itemCount})
			if err != nil {
				return err
			}
			frame.itemCount++
			frame.lastItem = item
			owner = item
		} else {
			lists = nil
		}
		kind := "paragraph"
		var attributes any = entity.KnowledgeDocumentEmptyAttributes{}
		if heading {
			kind = "heading"
			attributes = entity.KnowledgeDocumentHeadingAttributes{}
		}
		node, err := o.b.add(owner, kind, source.locator(), position, attributes)
		if err != nil {
			return err
		}
		node.own = true
		node.ownText = pptxInline(source)
	}
	return nil
}

func (o *documentOffice) table(parent *documentStructureNode, source *officeSourceElement, position entity.KnowledgeDocumentOriginalPosition, namespace string, rels map[string]officeSourceRelationship) error {
	rows := []*officeSourceElement{}
	maxCells := 0
	for _, row := range source.children {
		if row.is(namespace, "tr") {
			rows = append(rows, row)
			count := 0
			for _, cell := range row.children {
				if cell.is(namespace, "tc") {
					count++
				}
			}
			if count > maxCells {
				maxCells = count
			}
		}
	}
	if len(rows) > 8192 || maxCells > 8192 {
		return documentFailure("budget_exceeded")
	}
	table, err := o.b.add(parent, "table", source.locator(), position, entity.KnowledgeDocumentTableAttributes{RowCount: int64(len(rows)), MaxCellsPerRow: int64(maxCells), LayoutPrecision: "source_array"})
	if err != nil {
		return err
	}
	for rowIndex, rowSource := range rows {
		cells := []*officeSourceElement{}
		for _, cell := range rowSource.children {
			if cell.is(namespace, "tc") {
				cells = append(cells, cell)
			}
		}
		row, err := o.b.add(table, "row", rowSource.locator(), position, entity.KnowledgeDocumentRowAttributes{RowIndex: int64(rowIndex), CellCount: int64(len(cells))})
		if err != nil {
			return err
		}
		row.separator = "\t"
		for cellIndex, cellSource := range cells {
			cell, err := o.b.add(row, "cell", cellSource.locator(), position, entity.KnowledgeDocumentCellAttributes{RowIndex: int64(rowIndex), CellIndex: int64(cellIndex)})
			if err != nil {
				return err
			}
			if namespace == officeWordNS {
				if err := o.docxBlocks(cell, cellSource.children, rels); err != nil {
					return err
				}
			} else {
				body := cellSource.child(officeDrawingNS, "txBody")
				if body == nil {
					return ErrOfficePackage
				}
				if err := o.pptxText(cell, body, position, rels, false); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
