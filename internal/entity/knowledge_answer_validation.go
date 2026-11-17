package entity

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
)

// The generated, reference-free schema set comes from the exact approved KQA02
// subject and its unchanged C3 definitions. Validation uses the existing codec.
//
//go:embed knowledge_answer_schema.json
var knowledgeAnswerSchemaJSON []byte

var knowledgeAnswerSchemas = func() map[string]map[string]any {
	var schemas map[string]map[string]any
	decoder := json.NewDecoder(bytes.NewReader(knowledgeAnswerSchemaJSON))
	decoder.UseNumber()
	if err := decoder.Decode(&schemas); err != nil {
		panic(err)
	}
	return schemas
}()

func KnowledgeAnswerDecode(raw []byte, definition string, target any) error {
	if len(raw) > 1<<20 {
		return fmt.Errorf("answer JSON too large")
	}
	schema, ok := knowledgeAnswerSchemas[definition]
	if !ok {
		return fmt.Errorf("unknown answer definition")
	}
	value, err := knowledgeDocumentJSON(raw)
	if err != nil {
		return err
	}
	if err := knowledgeDocumentValidateSchema(schema, value, 0); err != nil {
		return err
	}
	if target != nil {
		normalized, err := json.Marshal(knowledgeAccessIntegers(value))
		if err != nil {
			return err
		}
		return json.Unmarshal(normalized, target)
	}
	return nil
}
