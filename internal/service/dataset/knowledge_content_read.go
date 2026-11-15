package dataset

import (
	"context"
	"errors"
	"unicode/utf8"

	"gorm.io/gorm"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func contentError(status int, code string) error {
	return knowledgeError(status, code, "Knowledge content request could not be completed")
}

func contentNormalize(err error) error {
	if err == nil {
		return nil
	}
	status := 503
	var knowledge *KnowledgeError
	var document *DocumentOperationError
	switch {
	case errors.As(err, &knowledge):
		status = knowledge.Status
	case errors.As(err, &document):
		status = document.Status
	case errors.Is(err, gorm.ErrRecordNotFound):
		status = 404
	}
	code := map[int]string{400: "invalid_request", 404: "not_found", 409: "content_integrity", 412: "contract_mismatch", 413: "capacity_exceeded", 415: "unsupported"}[status]
	if code == "" {
		status, code = 503, "unavailable"
	}
	return contentError(status, code)
}

func contentSameSource(ref entity.KnowledgeContentSourceRef, source *entity.KnowledgeSourceRevision) bool {
	return ref == contentSource(source)
}

func (d *DatasetService) GetKnowledgeContentRepresentation(ctx context.Context, userID, domainID string, target entity.KnowledgeContentTarget) (value *entity.KnowledgeContentRepresentation, err error) {
	defer func() { err = contentNormalize(err) }()
	kb, err := d.knowledgeBase(ctx, userID, domainID, true)
	if err != nil {
		return nil, err
	}
	value, err = d.contentStoredRepresentation(ctx, kb, target)
	if err != nil {
		return nil, err
	}
	if _, err = d.knowledgeBase(ctx, userID, domainID, true); err != nil {
		return nil, err
	}
	return value, nil
}

// contentStoredRepresentation verifies fixed storage identities. Its caller owns authorization.
func (d *DatasetService) contentStoredRepresentation(ctx context.Context, kb *entity.Knowledgebase, target entity.KnowledgeContentTarget) (value *entity.KnowledgeContentRepresentation, err error) {
	userID, domainID := kb.CreatedBy, kb.ID
	if target.Validate(domainID) != nil {
		return nil, contentError(400, "invalid_request")
	}
	target = contentTarget(target.Source, target.Representation.Kind, *target.Representation.ID, target.Representation.ProducerSHA256)
	value = &entity.KnowledgeContentRepresentation{Version: entity.KnowledgeContentVersion, Binding: "bound", Source: &target.Source, Ref: target.Representation, State: "ready"}
	switch target.Representation.Kind {
	case "source_revision":
		source, readErr := d.knowledgeSource(ctx, domainID, target.Source.RevisionID)
		if readErr != nil {
			var public *KnowledgeError
			if !errors.As(readErr, &public) || public.Status != 415 {
				return nil, readErr
			}
			original, originalErr := loadOriginalRevision(contentProofStore(ctx), domainID, target.Source.RevisionID)
			if originalErr != nil {
				var missing *KnowledgeError
				if errors.Is(originalErr, gorm.ErrRecordNotFound) || errors.As(originalErr, &missing) && missing.Status == 404 {
					source, parsedErr := parsedSourceRow(contentProofStore(ctx), kb, target.Source)
					if parsedErr == nil && source.RepresentationKind == "original_only" {
						value.State, value.Reason, value.NodeCount = "unavailable", contentString("original_has_no_text_representation"), contentInt(0)
						break
					}
				}
				return nil, originalErr
			}
			if original.DocumentID != target.Source.SourceID || original.SHA256 != target.Source.SHA256 {
				return nil, contentError(409, "content_integrity")
			}
			value.State, value.Reason, value.NodeCount = "unavailable", contentString("original_has_no_text_representation"), contentInt(0)
			break
		}
		if !contentSameSource(target.Source, source) {
			return nil, contentError(409, "content_integrity")
		}
		if source.RepresentationKind != "text" {
			return nil, contentError(415, "unsupported")
		}
		root := contentRef("c1_node", source.RootNodeID)
		value.RootRef, value.CoordinateSpace = &root, "source_utf8"
	case "structure_view":
		view, readErr := d.structureStoredView(ctx, userID, domainID, *target.Representation.ID)
		if readErr != nil {
			return nil, readErr
		}
		member, memberErr := structureMember(view, target.Source.SourceID)
		if memberErr != nil {
			return nil, memberErr
		}
		if member.SourceRevisionID != target.Source.RevisionID || member.SHA256 != target.Source.SHA256 || view.ProducerSHA256 != *target.Representation.ProducerSHA256 {
			return nil, contentError(409, "content_integrity")
		}
		root := contentRef("r1_node", member.RootStructureNodeID)
		value.RootRef, value.CoordinateSpace = &root, "source_utf8"
	case "parsed_representation":
		state, readErr := d.parsedStoredRepresentation(ctx, kb, *target.Representation.ID)
		if readErr != nil {
			return nil, readErr
		}
		rep := state.Representation
		if rep.Source != target.Source || *rep.Ref.ProducerSHA256 != *target.Representation.ProducerSHA256 {
			return nil, contentError(409, "content_integrity")
		}
		value.RootRef, value.CoordinateSpace, value.NodeCount, value.State = &rep.RootRef, "derived_utf8", contentInt(rep.NodeCount), rep.State
		if rep.State == "partial" {
			value.Reason = contentString("representation_contains_unsupported_content")
		}
	case "document_representation":
		rep, readErr := documentStoredRepresentation(contentProofStore(ctx), userID, domainID, *target.Representation.ID)
		if readErr != nil {
			return nil, readErr
		}
		if rep.Original.SourceID != target.Source.SourceID || rep.Original.SourceRevisionID != target.Source.RevisionID || rep.Original.SHA256 != target.Source.SHA256 || rep.ProducerSHA256 != *target.Representation.ProducerSHA256 {
			return nil, contentError(409, "content_integrity")
		}
		root := contentRef("r2_node", rep.RootDocumentNodeID)
		value.RootRef, value.CoordinateSpace, value.NodeCount = &root, "derived_utf8", contentInt(rep.NodeCount)
		if rep.Quality != "complete" || rep.UnsupportedBlockCount > 0 {
			value.State, value.Reason = "partial", contentString("representation_contains_unsupported_content")
		}
	}
	return value, nil
}

func (d *DatasetService) GetKnowledgeContentNode(ctx context.Context, userID, domainID string, address entity.KnowledgeContentAddress) (value *entity.KnowledgeContentNodeResult, err error) {
	defer func() { err = contentNormalize(err) }()
	if _, err = d.knowledgeBase(ctx, userID, domainID, true); err != nil {
		return nil, err
	}
	if address.Validate(domainID) != nil {
		return nil, contentError(400, "invalid_request")
	}
	target := contentTarget(address.Target.Source, address.Target.Representation.Kind, *address.Target.Representation.ID, address.Target.Representation.ProducerSHA256)
	meta, err := d.GetKnowledgeContentRepresentation(ctx, userID, domainID, target)
	if err != nil {
		return nil, err
	}
	if meta.RootRef == nil {
		return nil, contentError(415, "unsupported")
	}
	var projected *entity.KnowledgeContentNode
	switch target.Representation.Kind {
	case "source_revision":
		source, readErr := d.GetKnowledgeSource(ctx, userID, domainID, target.Source.RevisionID)
		if readErr != nil {
			return nil, readErr
		}
		node, readErr := d.GetKnowledgeNode(ctx, userID, domainID, *address.Node.ID)
		if readErr != nil {
			return nil, readErr
		}
		projected, err = contentProjectC1(source, node)
	case "structure_view":
		source, readErr := d.GetKnowledgeSource(ctx, userID, domainID, target.Source.RevisionID)
		if readErr != nil {
			return nil, readErr
		}
		view, readErr := d.GetKnowledgeStructureView(ctx, userID, domainID, *target.Representation.ID)
		if readErr != nil {
			return nil, readErr
		}
		node, readErr := d.GetKnowledgeStructureNode(ctx, userID, domainID, view.ViewID, *address.Node.ID)
		if readErr != nil {
			return nil, readErr
		}
		projected, err = contentProjectR1(source, view, node)
	case "parsed_representation":
		state, readErr := d.parsedReadRepresentation(ctx, userID, domainID, *target.Representation.ID)
		if readErr != nil {
			return nil, readErr
		}
		node, readErr := parsedReadNode(ctx, userID, domainID, state, *address.Node.ID)
		if readErr != nil {
			return nil, readErr
		}
		projected, err = parsedContentNode(state.Representation, node)
	case "document_representation":
		rep, readErr := d.GetKnowledgeDocumentRepresentation(ctx, userID, domainID, *target.Representation.ID)
		if readErr != nil {
			return nil, readErr
		}
		node, readErr := d.GetKnowledgeDocumentNode(ctx, userID, domainID, rep.RepresentationID, *address.Node.ID)
		if readErr != nil {
			return nil, readErr
		}
		projected, err = contentProjectR2(rep, node)
	}
	if err != nil {
		return nil, err
	}
	if _, err = d.GetKnowledgeContentRepresentation(ctx, userID, domainID, target); err != nil {
		return nil, err
	}
	return &entity.KnowledgeContentNodeResult{Version: entity.KnowledgeContentVersion, Target: target, Node: *projected}, nil
}

func (d *DatasetService) ListKnowledgeContentNodes(ctx context.Context, userID, domainID string, query entity.KnowledgeContentNodeQuery) (value *entity.KnowledgeContentNodePage, err error) {
	defer func() { err = contentNormalize(err) }()
	meta, err := d.GetKnowledgeContentRepresentation(ctx, userID, domainID, query.Target)
	if err != nil {
		return nil, err
	}
	if query.Limit < 1 || query.Limit > 100 {
		return nil, contentError(400, "invalid_request")
	}
	target := contentTarget(*meta.Source, meta.Ref.Kind, *meta.Ref.ID, meta.Ref.ProducerSHA256)
	value = &entity.KnowledgeContentNodePage{Version: entity.KnowledgeContentVersion, Target: target, Items: []entity.KnowledgeContentNode{}}
	parent := ""
	if query.Parent != nil {
		address := entity.KnowledgeContentAddress{Target: target, Node: *query.Parent}
		if address.Validate(domainID) != nil {
			return nil, contentError(400, "invalid_request")
		}
		if _, err = d.GetKnowledgeContentNode(ctx, userID, domainID, address); err != nil {
			return nil, err
		}
		parent = *query.Parent.ID
		copy := contentRef(query.Parent.Kind, parent)
		value.Parent = &copy
	}
	refs := []entity.KnowledgeContentNodeRef{}
	pageLimit := min(query.Limit, 20)
	innerCursor := query.Cursor
	var owningCursor *structureReadCursor
	owningScope := ""
	if meta.RootRef != nil && (target.Representation.Kind == "structure_view" || target.Representation.Kind == "document_representation" && query.Parent != nil) {
		owningScope = structureReadScope(userID, domainID, "content_nodes/0.2.0", target, parent, query.Limit, pageLimit)
		owningCursor, err = structureLoadReadCursor(ctx, userID, domainID, owningScope, query.Cursor, pageLimit)
		if err != nil {
			return nil, err
		}
		innerCursor = ""
		if query.Cursor != "" {
			// This adapter scope stores the owning API's opaque cursor in LastID.
			// Its full Target and original requested limit remain bound outside it.
			if !isKnowledgeID(owningCursor.LastID, 32) {
				return nil, contentError(400, "invalid_request")
			}
			innerCursor = owningCursor.LastID
		}
	}
	if meta.RootRef == nil {
		if query.Parent != nil || query.Cursor != "" {
			return nil, contentError(400, "invalid_request")
		}
	} else {
		switch target.Representation.Kind {
		case "source_revision":
			page, readErr := d.ListKnowledgeNodes(ctx, userID, domainID, target.Source.RevisionID, parent, query.Cursor, query.Limit)
			if readErr != nil {
				return nil, readErr
			}
			for _, node := range page.Items {
				refs = append(refs, contentRef("c1_node", node.ID))
			}
			if page.NextCursor != "" {
				value.NextCursor = contentString(page.NextCursor)
			}
		case "structure_view":
			page, readErr := d.ListKnowledgeStructureNodes(ctx, userID, domainID, *target.Representation.ID, target.Source.SourceID, parent, innerCursor, pageLimit)
			if readErr != nil {
				return nil, readErr
			}
			for _, node := range page.Items {
				refs = append(refs, contentRef("r1_node", node.StructureNodeID))
			}
			value.NextCursor = page.NextCursor
		case "parsed_representation":
			if query.Parent == nil {
				if query.Cursor != "" {
					return nil, contentError(400, "invalid_request")
				}
				refs = append(refs, *meta.RootRef)
			} else {
				scope := structureReadScope(userID, domainID, "parsed_nodes", *target.Representation.ID, parent, query.Limit)
				cursor, readErr := structureLoadReadCursor(ctx, userID, domainID, scope, query.Cursor, pageLimit)
				if readErr != nil {
					return nil, readErr
				}
				var rows []entity.KnowledgeDocumentNodeRecord
				if readErr := dao.DB.WithContext(ctx).Where("owner_id = ? AND domain_id = ? AND representation_id = ? AND parent_id = ?", userID, domainID, *target.Representation.ID, parent).Order("ordinal ASC, id ASC").Offset(cursor.Offset).Limit(pageLimit + 1).Find(&rows).Error; readErr != nil {
					return nil, readErr
				}
				for _, row := range rows[:min(pageLimit, len(rows))] {
					refs = append(refs, contentRef("parsed_node", row.ID))
				}
				if len(rows) > pageLimit {
					cursor.Offset += len(refs)
					value.NextCursor, err = structureSaveReadCursor(ctx, userID, domainID, scope, cursor)
					if err != nil {
						return nil, err
					}
				}
			}
		case "document_representation":
			if query.Parent == nil {
				if query.Cursor != "" {
					return nil, contentError(400, "invalid_request")
				}
				refs = append(refs, *meta.RootRef)
			} else {
				page, readErr := d.ListKnowledgeDocumentNodes(ctx, userID, domainID, *target.Representation.ID, parent, innerCursor, pageLimit)
				if readErr != nil {
					return nil, readErr
				}
				for _, node := range page.Items {
					refs = append(refs, contentRef("r2_node", node.DocumentNodeID))
				}
				value.NextCursor = page.NextCursor
			}
		}
	}
	if value.NextCursor != nil && owningCursor != nil {
		owningCursor.LastID = *value.NextCursor
		value.NextCursor, err = structureSaveReadCursor(ctx, userID, domainID, owningScope, owningCursor)
		if err != nil {
			return nil, err
		}
	}
	for _, ref := range refs {
		node, readErr := d.GetKnowledgeContentNode(ctx, userID, domainID, entity.KnowledgeContentAddress{Target: target, Node: ref})
		if readErr != nil {
			return nil, readErr
		}
		value.Items = append(value.Items, node.Node)
	}
	if _, err = d.GetKnowledgeContentRepresentation(ctx, userID, domainID, target); err != nil {
		return nil, err
	}
	return value, nil
}

func (d *DatasetService) ReadKnowledgeContentNode(ctx context.Context, userID, domainID string, request entity.KnowledgeContentReadRequest) (value *entity.KnowledgeContentWindow, err error) {
	defer func() { err = contentNormalize(err) }()
	result, err := d.GetKnowledgeContentNode(ctx, userID, domainID, request.Address)
	if err != nil {
		return nil, err
	}
	if request.EndByte != nil && request.StartByte == nil {
		return nil, contentError(400, "invalid_request")
	}
	address := entity.KnowledgeContentAddress{Target: result.Target, Node: result.Node.Ref}
	node := result.Node
	if node.Text.Range == nil {
		return nil, contentError(415, "unsupported")
	}
	from := node.Text.Range.StartByte
	if request.StartByte != nil {
		from = *request.StartByte
	}
	if from < node.Text.Range.StartByte || from > node.Text.Range.EndByte {
		return nil, contentError(400, "invalid_request")
	}
	if request.EndByte != nil && (*request.EndByte < from || *request.EndByte > node.Text.Range.EndByte || *request.EndByte-from > 65536) {
		return nil, contentError(400, "invalid_request")
	}
	var start, end int64
	var text string
	var complete bool
	var next *int64
	switch address.Target.Representation.Kind {
	case "source_revision":
		source, readErr := d.GetKnowledgeSource(ctx, userID, domainID, address.Target.Source.RevisionID)
		if readErr != nil {
			return nil, readErr
		}
		body, readErr := knowledgeSourceBytes(ctx, source)
		if readErr != nil {
			return nil, readErr
		}
		start, end = node.Text.Range.StartByte, node.Text.Range.EndByte
		if request.StartByte != nil {
			start = *request.StartByte
		}
		if request.EndByte != nil {
			end = *request.EndByte
		} else {
			end = min(end, start+65536)
			for end > start && end >= 0 && end < int64(len(body)) && !utf8.RuneStart(body[end]) {
				end--
			}
		}
		if start < node.Text.Range.StartByte || end > node.Text.Range.EndByte || end-start > 65536 || !knowledgeRange(body, start, end) || start == end && node.Text.Status == "available" {
			return nil, contentError(400, "invalid_request")
		}
		text, complete = string(body[start:end]), end == node.Text.Range.EndByte
		if !complete {
			next = contentInt(end)
		}
	case "structure_view":
		window, readErr := d.ReadKnowledgeStructureNode(ctx, userID, domainID, *address.Target.Representation.ID, *address.Node.ID, request.StartByte, request.EndByte)
		if readErr != nil {
			return nil, readErr
		}
		start, end, text, complete, next = window.StartByte, window.EndByte, window.Text, window.Complete, window.NextStartByte
	case "parsed_representation":
		window, readErr := d.parsedReadContent(ctx, userID, domainID, address, request.StartByte, request.EndByte)
		if readErr != nil {
			return nil, readErr
		}
		start, end, text, complete, next = window.Range.StartByte, window.Range.EndByte, window.Text, window.Complete, window.NextStartByte
	case "document_representation":
		window, readErr := d.ReadKnowledgeDocumentNode(ctx, userID, domainID, *address.Target.Representation.ID, *address.Node.ID, request.StartByte, request.EndByte)
		if readErr != nil {
			return nil, readErr
		}
		start, end, text, complete, next = window.StartByte, window.EndByte, window.Text, window.Complete, window.NextStartByte
	}
	if start < node.Text.Range.StartByte || end > node.Text.Range.EndByte || end < start || int64(len(text)) != end-start || end-start > 65536 || !utf8.ValidString(text) {
		return nil, contentError(409, "content_integrity")
	}
	if start == end && node.Text.Status == "available" {
		return nil, contentError(400, "invalid_request")
	}
	if _, err = d.GetKnowledgeContentRepresentation(ctx, userID, domainID, address.Target); err != nil {
		return nil, err
	}
	return &entity.KnowledgeContentWindow{Version: entity.KnowledgeContentVersion, Address: address, Locator: node.Locator, CoordinateSpace: node.Text.CoordinateSpace, Range: entity.KnowledgeContentRange{StartByte: start, EndByte: end, SHA256: contentString(directorySHA([]byte(text)))}, TextStatus: node.Text.Status, TextReason: node.Text.Reason, Text: text, Complete: complete, NextStartByte: next}, nil
}
