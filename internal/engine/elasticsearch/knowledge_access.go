package elasticsearch

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"

	"ragflow/internal/engine/types"
)

// The established source.id keyword is the exact sortable row identity. The
// template alone cannot establish the mapping of an already-created index.
var knowledgeAccessIndexPattern = regexp.MustCompile("^ragflow_[a-f0-9]{32}$")

func (e *Engine) verifyKnowledgeAccessSearch(ctx context.Context, req *types.SearchRequest) error {
	fail := func() error { return fmt.Errorf("scoped lexical search unavailable") }
	validHash := func(value string) bool {
		raw, err := hex.DecodeString(value)
		return err == nil && len(raw) == 32 && strings.ToLower(value) == value
	}
	if len(req.IndexNames) != 1 || len(req.KbIDs) != 1 || req.Offset != 0 || req.Limit < 1 || req.Limit > 20 || len(req.RankFeature) != 0 || req.Filter["available_int"] != 0 || req.Filter["knowledge_available_exact"] != 0 || !knowledgeAccessIndexPattern.MatchString(req.IndexNames[0]) {
		return fail()
	}
	clauses, ok := req.Filter["or"].([]map[string]any)
	if !ok || len(clauses) == 0 || len(clauses) > 32 {
		return fail()
	}
	count := 0
	for _, clause := range clauses {
		binding, ok := clause["knowledge_binding_kwd"].(string)
		ids, idsOK := clause["id"].([]string)
		if !ok || !idsOK || len(clause) != 2 || len(ids) == 0 || !validHash(binding) {
			return fail()
		}
		for _, id := range ids {
			if !validHash(id) {
				return fail()
			}
			count++
		}
	}
	if count > 32768 || len(req.MatchExprs) != 1 {
		return fail()
	}
	if req.MatchExprs[0] != nil {
		if _, ok := req.MatchExprs[0].(*types.MatchTextExpr); !ok {
			return fail()
		}
	} else if req.Filter["knowledge_match_none"] != true {
		return fail()
	}
	response, err := e.client.Indices.GetMapping(e.client.Indices.GetMapping.WithContext(ctx), e.client.Indices.GetMapping.WithIndex(req.IndexNames[0]))
	if err != nil {
		return fail()
	}
	defer response.Body.Close()
	if response.IsError() {
		return fail()
	}
	var mapping map[string]struct {
		Mappings struct {
			Properties map[string]map[string]any `json:"properties"`
		} `json:"mappings"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&mapping) != nil || len(mapping) != 1 {
		return fail()
	}
	entry, ok := mapping[req.IndexNames[0]]
	if !ok {
		return fail()
	}
	id := entry.Mappings.Properties["id"]
	if id["type"] != "keyword" || id["doc_values"] == false || id["index"] == false || id["normalizer"] != nil {
		return fail()
	}
	if above, ok := id["ignore_above"].(float64); ok && above < 64 {
		return fail()
	}
	return nil
}
