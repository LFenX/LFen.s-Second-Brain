package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/elastic/go-elasticsearch/v8/esapi"
	"io"
	"net/http"
	"strings"
)

const knowledgeWriteGuard = "if (ctx._source.containsKey('knowledge_binding_kwd') || ctx._source.containsKey('knowledge_ref')) { throw new IllegalArgumentException('immutable knowledge row'); } "

func knowledgeReserved(m map[string]any) bool {
	_, a := m["knowledge_binding_kwd"]
	_, b := m["knowledge_ref"]
	return a || b
}
func knowledgeForbiddenPayload(m map[string]any) bool {
	for k := range m {
		if k == "mom_id" || k == "parent_id" || strings.HasPrefix(k, "q_") && strings.HasSuffix(k, "_vec") {
			return true
		}
	}
	return false
}
func knowledgeDecode(res *esapi.Response, out any) error {
	defer res.Body.Close()
	if res.IsError() {
		return fmt.Errorf("knowledge ES request: HTTP %d", res.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(res.Body, 64<<20)).Decode(out)
}

// PrepareKnowledgeLexical verifies existing mapping types before adding only
// the two approved fields. A conflict never deletes or recreates an index.
func (e *Engine) PrepareKnowledgeLexical(ctx context.Context, index, dataset, language string) error {
	if !strings.HasPrefix(index, "ragflow_") {
		return errors.New("invalid knowledge index")
	}
	if err := e.CreateChunkStore(ctx, index, dataset, 0, "general", language); err != nil {
		return err
	}
	res, err := e.client.Indices.GetMapping(e.client.Indices.GetMapping.WithContext(ctx), e.client.Indices.GetMapping.WithIndex(index))
	if err != nil {
		return err
	}
	var existing map[string]struct {
		Mappings struct {
			Properties map[string]map[string]any `json:"properties"`
		} `json:"mappings"`
	}
	if err := knowledgeDecode(res, &existing); err != nil {
		return err
	}
	for _, entry := range existing {
		for _, name := range []string{"knowledge_ref", "knowledge_binding_kwd"} {
			if f, ok := entry.Mappings.Properties[name]; ok {
				if f["type"] != "keyword" {
					return errors.New("knowledge mapping type conflict")
				}
				if name == "knowledge_ref" && (f["index"] != false || f["doc_values"] != false) {
					return errors.New("knowledge reference mapping conflict")
				}
				if name == "knowledge_binding_kwd" && f["index"] == false {
					return errors.New("knowledge binding mapping conflict")
				}
			}
		}
	}
	body, _ := json.Marshal(map[string]any{"properties": map[string]any{"knowledge_ref": map[string]any{"type": "keyword", "index": false, "doc_values": false}, "knowledge_binding_kwd": map[string]any{"type": "keyword"}}})
	res, err = e.client.Indices.PutMapping([]string{index}, bytes.NewReader(body), e.client.Indices.PutMapping.WithContext(ctx))
	if err != nil {
		return err
	}
	var ack struct {
		Acknowledged bool `json:"acknowledged"`
	}
	if err := knowledgeDecode(res, &ack); err != nil {
		return err
	}
	if !ack.Acknowledged {
		return errors.New("knowledge mapping not acknowledged")
	}
	return nil
}

func knowledgeEquivalent(a, b map[string]any) bool {
	copy := func(x map[string]any) map[string]any {
		r := map[string]any{}
		for k, v := range x {
			if k != "create_time" && k != "create_timestamp_flt" {
				r[k] = v
			}
		}
		return r
	}
	x, _ := json.Marshal(copy(a))
	y, _ := json.Marshal(copy(b))
	return bytes.Equal(x, y)
}
func (e *Engine) knowledgeGet(ctx context.Context, index, id string) (map[string]any, error) {
	res, err := e.client.Get(index, id, e.client.Get.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	var doc struct {
		ID     string         `json:"_id"`
		Found  bool           `json:"found"`
		Source map[string]any `json:"_source"`
	}
	if err := knowledgeDecode(res, &doc); err != nil {
		return nil, err
	}
	if !doc.Found || doc.ID != id {
		return nil, errors.New("knowledge row missing")
	}
	return doc.Source, nil
}

// CreateKnowledgeLexicalRows uses create, never upsert. A conflict is accepted
// only after comparing the complete persisted projection with the same row.
func (e *Engine) CreateKnowledgeLexicalRows(ctx context.Context, index, dataset string, rows []map[string]any) error {
	for start := 0; start < len(rows); start += 128 {
		end := min(start+128, len(rows))
		var body bytes.Buffer
		for _, row := range rows[start:end] {
			id, ok := row["id"].(string)
			ref, refOK := row["knowledge_ref"].(string)
			binding, bindOK := row["knowledge_binding_kwd"].(string)
			if !ok || len(id) != 64 || !refOK || ref == "" || !bindOK || len(binding) != 64 || row["available_int"] != 0 || row["kb_id"] != dataset || knowledgeForbiddenPayload(row) {
				return errors.New("invalid knowledge row")
			}
			action, _ := json.Marshal(map[string]any{"create": map[string]any{"_index": index, "_id": id}})
			body.Write(action)
			body.WriteByte('\n')
			raw, err := json.Marshal(row)
			if err != nil {
				return err
			}
			body.Write(raw)
			body.WriteByte('\n')
		}
		res, err := e.client.Bulk(bytes.NewReader(body.Bytes()), e.client.Bulk.WithContext(ctx), e.client.Bulk.WithRefresh("wait_for"))
		if err != nil {
			return err
		}
		var result struct {
			Items []map[string]struct {
				Status int    `json:"status"`
				ID     string `json:"_id"`
			} `json:"items"`
		}
		if err := knowledgeDecode(res, &result); err != nil {
			return err
		}
		if len(result.Items) != end-start {
			return errors.New("incomplete knowledge bulk receipt")
		}
		for offset, item := range result.Items {
			op, ok := item["create"]
			row := rows[start+offset]
			if !ok || op.ID != row["id"] {
				return errors.New("knowledge bulk identity mismatch")
			}
			if op.Status == http.StatusConflict {
				existing, err := e.knowledgeGet(ctx, index, op.ID)
				if err != nil {
					return err
				}
				if !knowledgeEquivalent(existing, row) {
					return errors.New("knowledge row conflict is not equivalent")
				}
			} else if op.Status != http.StatusCreated {
				return errors.New("knowledge row creation failed")
			}
		}
	}
	return e.VerifyKnowledgeLexicalRows(ctx, index, dataset, rows)
}

// VerifyKnowledgeLexicalRows checks every expected row and the complete binding
// cardinality, including hidden/malformed availability rows. No partial set passes.
func (e *Engine) VerifyKnowledgeLexicalRows(ctx context.Context, index, dataset string, rows []map[string]any) error {
	bindings := map[string]bool{}
	seen := map[string]bool{}
	for start := 0; start < len(rows); start += 128 {
		end := min(start+128, len(rows))
		ids := []string{}
		for _, row := range rows[start:end] {
			id, _ := row["id"].(string)
			binding, _ := row["knowledge_binding_kwd"].(string)
			if id == "" || binding == "" || seen[id] {
				return errors.New("invalid knowledge row manifest")
			}
			seen[id] = true
			ids = append(ids, id)
			bindings[binding] = true
		}
		raw, _ := json.Marshal(map[string]any{"ids": ids})
		res, err := e.client.Mget(bytes.NewReader(raw), e.client.Mget.WithContext(ctx), e.client.Mget.WithIndex(index))
		if err != nil {
			return err
		}
		var result struct {
			Docs []struct {
				ID     string         `json:"_id"`
				Found  bool           `json:"found"`
				Source map[string]any `json:"_source"`
			} `json:"docs"`
		}
		if err := knowledgeDecode(res, &result); err != nil {
			return err
		}
		if len(result.Docs) != len(ids) {
			return errors.New("knowledge row receipt incomplete")
		}
		for i, doc := range result.Docs {
			if !doc.Found || doc.ID != ids[i] || knowledgeForbiddenPayload(doc.Source) {
				return errors.New("knowledge row missing or malformed")
			}
			for _, key := range []string{"id", "kb_id", "doc_id", "knowledge_binding_kwd", "knowledge_ref", "content_with_weight", "available_int"} {
				x, _ := json.Marshal(doc.Source[key])
				y, _ := json.Marshal(rows[start+i][key])
				if !bytes.Equal(x, y) {
					return fmt.Errorf("knowledge row field mismatch: %s", key)
				}
			}
		}
	}
	if len(rows) == 0 {
		return nil
	}
	keys := []string{}
	for b := range bindings {
		keys = append(keys, b)
	}
	raw, _ := json.Marshal(map[string]any{"size": 0, "track_total_hits": true, "query": map[string]any{"bool": map[string]any{"filter": []any{map[string]any{"term": map[string]any{"kb_id": dataset}}, map[string]any{"terms": map[string]any{"knowledge_binding_kwd": keys}}}}}})
	res, err := e.client.Search(e.client.Search.WithContext(ctx), e.client.Search.WithIndex(index), e.client.Search.WithBody(bytes.NewReader(raw)))
	if err != nil {
		return err
	}
	var count struct {
		Hits struct {
			Total struct {
				Value    int    `json:"value"`
				Relation string `json:"relation"`
			} `json:"total"`
		} `json:"hits"`
	}
	if err := knowledgeDecode(res, &count); err != nil {
		return err
	}
	if count.Hits.Total.Relation != "eq" || count.Hits.Total.Value != len(rows) {
		return errors.New("knowledge binding row set mismatch")
	}
	return nil
}

func knowledgeOrdinaryUpdate(row map[string]any) map[string]any {
	return map[string]any{"scripted_upsert": true, "script": map[string]any{"lang": "painless", "source": knowledgeWriteGuard + "ctx._source.clear(); ctx._source.putAll(params.doc);", "params": map[string]any{"doc": row}}, "upsert": map[string]any{}}
}
