package dataset

import (
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"ragflow/internal/entity"
	structure "ragflow/internal/parser/structure"
)

type structureBoundBuild struct {
	View            entity.KnowledgeStructureView
	Nodes           []entity.KnowledgeStructureNodeRecord
	Relations       []entity.KnowledgeStructureRelationRecord
	MemberNodes     map[string]int64
	MemberRelations map[string]int64
	Diagnostics     map[string][]entity.KnowledgeStructureDiagnostic
}

func structureViewDigest(owner, domain, project, producer string, inputs *structureInputs) (string, error) {
	members := make([]map[string]any, 0, len(inputs.Members))
	for _, member := range inputs.Members {
		members = append(members, map[string]any{"source_id": member.SourceID, "source_revision_id": member.SourceRevisionID, "lifecycle_version": member.LifecycleVersion, "sha256": member.SHA256, "classification": member.Classification, "relative_path": member.RelativePath})
	}
	slices.SortFunc(members, func(a, b map[string]any) int {
		return strings.Compare(a["source_id"].(string), b["source_id"].(string))
	})
	return entity.KnowledgeLifecycleDigest(map[string]any{"kind": "knowledge-structure-view/0.1.0", "owner_id": owner, "domain_id": domain, "project_id": project, "manifest_id": inputs.ManifestID, "manifest_sha256": inputs.ManifestSHA256, "foundation_policy_version": inputs.FoundationPolicyVersion, "structure_policy_version": inputs.StructurePolicyVersion, "producer_sha256": producer, "members": members})
}

func structureLogicalDigest(domain, source, kind, key string) (string, error) {
	return entity.KnowledgeLifecycleDigest(map[string]any{"kind": "knowledge-structure-logical/0.1.0", "domain_id": domain, "source_id": source, "node_kind": kind, "logical_key": key})
}

func structureNodeDigest(view, logical, revision string, start, end int64) (string, error) {
	return entity.KnowledgeLifecycleDigest(map[string]any{"kind": "knowledge-structure-node/0.1.0", "view_id": view, "logical_node_id": logical, "source_revision_id": revision, "start_byte": start, "end_byte": end})
}

func structureRelationDigest(relation entity.KnowledgeStructureRelation) (string, error) {
	return entity.KnowledgeLifecycleDigest(map[string]any{"kind": "knowledge-structure-relation/0.1.0", "view_id": relation.ViewID, "relation_kind": relation.Kind, "from_node_id": relation.FromNodeID, "to_node_id": relation.ToNodeID, "reference": relation.Reference, "evidence": relation.Evidence})
}

func structureRefKey(source, key string) string { return source + "\x00" + key }

func structurePhysicalLines(lines []int, start, end int) (int64, int64) {
	first := int64(sort.SearchInts(lines, start+1))
	last := first
	if end > start {
		last = int64(sort.SearchInts(lines, end))
	}
	return first, last
}

func structureTextBounded(value string, min, max int) bool {
	return utf8.ValidString(value) && len(value) >= min && len(value) <= max
}

func structureValidateFile(file structure.File, member entity.KnowledgeStructureViewMember, raw []byte) error {
	if file.SourceID != member.SourceID || file.RevisionID != member.SourceRevisionID || file.Path != member.RelativePath || file.Parser != member.Parser || file.SHA256 != member.SHA256 || int64(file.ByteLength) != member.ByteLength || int64(len(raw)) != member.ByteLength || len(raw) > 2<<20 || !utf8.Valid(raw) || directorySHA(raw) != member.SHA256 || len(file.Nodes) < 1 || len(file.Nodes) > 8192 || len(file.Diagnostics) > 8 {
		return structureError(409, "structure_integrity")
	}
	nodes := make(map[string]structure.Node, len(file.Nodes))
	lines := []int{0}
	for i, character := range raw {
		if character == '\n' {
			lines = append(lines, i+1)
		}
	}
	roots := 0
	passages := []structure.Node{}
	for _, node := range file.Nodes {
		if node.Key == "" || !structureTextBounded(node.Name, 0, 256) || !structureTextBounded(node.LogicalKey, 1, 512) || !slices.Contains([]string{"file", "symbol", "section", "passage"}, node.Kind) || !slices.Contains([]string{"file", "name_scope", "position_based", "range_based"}, node.IdentityStability) || !knowledgeRange(raw, int64(node.StartByte), int64(node.EndByte)) {
			return structureError(409, "structure_integrity")
		}
		if _, duplicate := nodes[node.Key]; duplicate {
			return structureError(409, "structure_integrity")
		}
		nodes[node.Key] = node
		startLine, endLine := structurePhysicalLines(lines, node.StartByte, node.EndByte)
		if int64(node.StartLine) != startLine || int64(node.EndLine) != endLine {
			return structureError(409, "structure_integrity")
		}
		if node.Kind == "file" {
			roots++
			if node.ParentKey != "" || node.StartByte != 0 || node.EndByte != len(raw) || node.IdentityStability != "file" {
				return structureError(409, "structure_integrity")
			}
		} else if node.ParentKey == "" {
			return structureError(409, "structure_integrity")
		}
		if node.Kind == "passage" {
			if node.EndByte <= node.StartByte || node.EndByte-node.StartByte > 8192 {
				return structureError(409, "structure_integrity")
			}
			passages = append(passages, node)
		}
	}
	if roots != 1 {
		return structureError(409, "structure_integrity")
	}
	for _, node := range file.Nodes {
		cursor := node
		seen := map[string]bool{node.Key: true}
		for depth := 1; cursor.ParentKey != ""; depth++ {
			parent, exists := nodes[cursor.ParentKey]
			if !exists || depth >= 16 || seen[parent.Key] || parent.Kind == "passage" || parent.StartByte > cursor.StartByte || parent.EndByte < cursor.EndByte {
				return structureError(409, "structure_integrity")
			}
			seen[parent.Key] = true
			cursor = parent
		}
		if cursor.Kind != "file" {
			return structureError(409, "structure_integrity")
		}
		if node.Kind == "passage" {
			parent := nodes[node.ParentKey]
			width := parent.EndByte - parent.StartByte
			for _, semantic := range file.Nodes {
				if semantic.Kind != "passage" && semantic.StartByte <= node.StartByte && semantic.EndByte >= node.EndByte && semantic.EndByte-semantic.StartByte < width {
					return structureError(409, "structure_integrity")
				}
			}
		}
	}
	slices.SortFunc(passages, func(a, b structure.Node) int {
		if a.StartByte != b.StartByte {
			return a.StartByte - b.StartByte
		}
		return a.EndByte - b.EndByte
	})
	position := 0
	for _, passage := range passages {
		if passage.StartByte != position {
			return structureError(409, "structure_integrity")
		}
		position = passage.EndByte
	}
	if position != len(raw) {
		return structureError(409, "structure_integrity")
	}
	for _, diagnostic := range file.Diagnostics {
		if !structureTextBounded(diagnostic.Code, 1, 64) || !structureTextBounded(diagnostic.Message, 0, 128) || (diagnostic.StartByte == nil) != (diagnostic.EndByte == nil) || diagnostic.StartByte != nil && !knowledgeRange(raw, int64(*diagnostic.StartByte), int64(*diagnostic.EndByte)) {
			return structureError(409, "structure_integrity")
		}
	}
	return nil
}

func structureBindBuild(owner, domain, project, producer string, inputs *structureInputs, output structure.Output, source map[string][]byte) (*structureBoundBuild, error) {
	if len(output.Files) != len(inputs.Members) || len(output.Files) < 1 || len(output.Files) > 32 {
		return nil, structureError(409, "structure_integrity")
	}
	viewID, err := structureViewDigest(owner, domain, project, producer, inputs)
	if err != nil {
		return nil, err
	}
	bound := &structureBoundBuild{View: entity.KnowledgeStructureView{ViewID: viewID, DomainID: domain, OwnerID: owner, ProjectID: project, ManifestID: inputs.ManifestID, ManifestSHA256: inputs.ManifestSHA256, FoundationPolicyVersion: inputs.FoundationPolicyVersion, StructurePolicyVersion: inputs.StructurePolicyVersion, ProducerSHA256: producer, ProducerFormat: "code-structure-v1", Aliases: slices.Clone(inputs.Aliases), Members: slices.Clone(inputs.Members), SourceCount: int64(len(inputs.Members)), LexicalIndex: "not_created", VectorIndex: "not_created"}, Nodes: []entity.KnowledgeStructureNodeRecord{}, Relations: []entity.KnowledgeStructureRelationRecord{}, MemberNodes: map[string]int64{}, MemberRelations: map[string]int64{}, Diagnostics: map[string][]entity.KnowledgeStructureDiagnostic{}}
	memberByID := make(map[string]entity.KnowledgeStructureViewMember, len(inputs.Members))
	for _, member := range inputs.Members {
		if _, duplicate := memberByID[member.SourceID]; duplicate {
			return nil, structureError(409, "structure_integrity")
		}
		memberByID[member.SourceID] = member
		bound.View.SourceBytes += member.ByteLength
	}
	if bound.View.SourceBytes > 16<<20 {
		return nil, structureError(409, "structure_integrity")
	}
	seenFiles := map[string]bool{}
	nodes := map[string]entity.KnowledgeStructureNode{}
	nodeIDs := map[string]bool{}
	localParents := map[string]string{}
	fileRoots := map[string]string{}
	fragmentSHA := map[string]string{}
	for _, file := range output.Files {
		member, exists := memberByID[file.SourceID]
		if !exists || seenFiles[file.SourceID] {
			return nil, structureError(409, "structure_integrity")
		}
		seenFiles[file.SourceID] = true
		raw, exists := source[file.SourceID]
		if !exists || structureValidateFile(file, member, raw) != nil {
			return nil, structureError(409, "structure_integrity")
		}
		bound.Diagnostics[file.SourceID] = []entity.KnowledgeStructureDiagnostic{}
		for _, diagnostic := range file.Diagnostics {
			item := entity.KnowledgeStructureDiagnostic{Code: diagnostic.Code, Message: diagnostic.Message}
			if diagnostic.StartByte != nil {
				start, end := int64(*diagnostic.StartByte), int64(*diagnostic.EndByte)
				item.StartByte, item.EndByte = &start, &end
			}
			bound.Diagnostics[file.SourceID] = append(bound.Diagnostics[file.SourceID], item)
		}
		for _, produced := range file.Nodes {
			logicalID, err := structureLogicalDigest(domain, file.SourceID, produced.Kind, produced.LogicalKey)
			if err != nil {
				return nil, err
			}
			nodeID, err := structureNodeDigest(viewID, logicalID, member.SourceRevisionID, int64(produced.StartByte), int64(produced.EndByte))
			if err != nil {
				return nil, err
			}
			if nodeIDs[nodeID] {
				return nil, structureError(409, "structure_integrity")
			}
			nodeIDs[nodeID] = true
			key := structureRefKey(file.SourceID, produced.Key)
			nodes[key] = entity.KnowledgeStructureNode{StructureNodeID: nodeID, LogicalNodeID: logicalID, ViewID: viewID, SourceID: file.SourceID, SourceRevisionID: member.SourceRevisionID, SourceNodeID: member.SourceNodeID, Kind: produced.Kind, Name: produced.Name, LogicalKey: produced.LogicalKey, IdentityStability: produced.IdentityStability, StartByte: int64(produced.StartByte), EndByte: int64(produced.EndByte), StartLine: int64(produced.StartLine), EndLine: int64(produced.EndLine), Parser: member.Parser, Classification: member.Classification, CoordinateSpace: "source_utf8", ProducerSHA256: producer}
			fragmentSHA[key] = directorySHA(raw[produced.StartByte:produced.EndByte])
			localParents[key] = produced.ParentKey
			bound.MemberNodes[file.SourceID]++
			bound.View.NodeCount++
			if produced.Kind == "file" {
				fileRoots[file.SourceID] = nodeID
			}
		}
	}
	if bound.View.NodeCount > 32768 {
		return nil, structureError(409, "structure_integrity")
	}
	for key, node := range nodes {
		if localParents[key] != "" {
			parent, exists := nodes[structureRefKey(node.SourceID, localParents[key])]
			if !exists {
				return nil, structureError(409, "structure_integrity")
			}
			node.ParentID = &parent.StructureNodeID
			bound.MemberRelations[node.SourceID]++
			bound.View.RelationCount++
		}
		nodes[key] = node
		payload, err := structureJSON(node)
		if err != nil {
			return nil, err
		}
		parent := ""
		if node.ParentID != nil {
			parent = *node.ParentID
		}
		bound.Nodes = append(bound.Nodes, entity.KnowledgeStructureNodeRecord{ID: node.StructureNodeID, OwnerID: owner, DomainID: domain, ViewID: viewID, SourceID: node.SourceID, SourceRevisionID: node.SourceRevisionID, ParentID: parent, Kind: node.Kind, StartByte: node.StartByte, EndByte: node.EndByte, FragmentSHA256: fragmentSHA[key], Payload: payload})
	}
	for i := range bound.View.Members {
		bound.View.Members[i].RootStructureNodeID = fileRoots[bound.View.Members[i].SourceID]
	}
	seenRelations := map[string]bool{}
	for _, file := range output.Files {
		for _, produced := range file.Relations {
			from, exists := nodes[structureRefKey(produced.From.SourceID, produced.From.Key)]
			if !exists || from.SourceID != file.SourceID || !slices.Contains([]string{"imports_file", "exports_symbol", "references_type", "calls_local"}, produced.Kind) || !structureTextBounded(produced.Reference, 0, 512) {
				return nil, structureError(409, "structure_integrity")
			}
			member, exists := memberByID[produced.Evidence.SourceID]
			raw := source[produced.Evidence.SourceID]
			if !exists || produced.Evidence.SourceID != from.SourceID || produced.Evidence.RevisionID != member.SourceRevisionID || !knowledgeRange(raw, int64(produced.Evidence.StartByte), int64(produced.Evidence.EndByte)) || produced.Evidence.FragmentSHA256 != directorySHA(raw[produced.Evidence.StartByte:produced.Evidence.EndByte]) {
				return nil, structureError(409, "structure_integrity")
			}
			relation := entity.KnowledgeStructureRelation{ViewID: viewID, Kind: produced.Kind, FromNodeID: from.StructureNodeID, Resolution: produced.Resolution, Reason: produced.Reason, Reference: produced.Reference, ReferenceTruncated: produced.ReferenceTruncated, Evidence: entity.KnowledgeStructureEvidence{SourceID: produced.Evidence.SourceID, SourceRevisionID: member.SourceRevisionID, StartByte: int64(produced.Evidence.StartByte), EndByte: int64(produced.Evidence.EndByte), FragmentSHA256: produced.Evidence.FragmentSHA256}}
			if produced.Resolution == "resolved" {
				if produced.To == nil || !slices.Contains([]string{"explicit_relative", "explicit_alias", "named_export", "lexical_binding"}, produced.Reason) {
					return nil, structureError(409, "structure_integrity")
				}
				target, exists := nodes[structureRefKey(produced.To.SourceID, produced.To.Key)]
				if !exists {
					return nil, structureError(409, "structure_integrity")
				}
				relation.ToNodeID = &target.StructureNodeID
			} else if produced.Resolution != "unresolved" || produced.To != nil || !slices.Contains([]string{"external_package", "outside_selection", "ambiguous", "dynamic_import", "dynamic_call", "wildcard_export", "cycle", "unsupported_target", "go_package_unresolved", "reference_too_long"}, produced.Reason) {
				return nil, structureError(409, "structure_integrity")
			}
			id, err := structureRelationDigest(relation)
			if err != nil {
				return nil, err
			}
			if seenRelations[id] {
				return nil, structureError(409, "structure_integrity")
			}
			seenRelations[id] = true
			relation.RelationID = id
			payload, err := structureJSON(relation)
			if err != nil {
				return nil, err
			}
			target := ""
			if relation.ToNodeID != nil {
				target = *relation.ToNodeID
			}
			bound.Relations = append(bound.Relations, entity.KnowledgeStructureRelationRecord{ID: id, OwnerID: owner, DomainID: domain, ViewID: viewID, FromNodeID: relation.FromNodeID, ToNodeID: target, Kind: relation.Kind, Payload: payload})
			bound.View.RelationCount++
			bound.MemberRelations[file.SourceID]++
		}
	}
	if bound.View.RelationCount > 65536 {
		return nil, structureError(409, "structure_integrity")
	}
	slices.SortFunc(bound.Nodes, func(a, b entity.KnowledgeStructureNodeRecord) int {
		if a.SourceID != b.SourceID {
			return strings.Compare(a.SourceID, b.SourceID)
		}
		if a.StartByte != b.StartByte {
			return int(a.StartByte - b.StartByte)
		}
		if a.EndByte != b.EndByte {
			return int(a.EndByte - b.EndByte)
		}
		if a.Kind != b.Kind {
			return strings.Compare(a.Kind, b.Kind)
		}
		return strings.Compare(a.ID, b.ID)
	})
	slices.SortFunc(bound.Relations, func(a, b entity.KnowledgeStructureRelationRecord) int { return strings.Compare(a.ID, b.ID) })
	return bound, nil
}
