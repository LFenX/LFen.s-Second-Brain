package entity

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"runtime"
	"slices"
)

//go:embed knowledge_structure_capabilities.json
var KnowledgeStructureCapabilitiesJSON []byte

//go:embed knowledge_structure_producer.json
var KnowledgeStructureProducerJSON []byte

// KnowledgeStructureProducerCompatible prevents a different standard Go parser
// from silently claiming the frozen producer identity.
func KnowledgeStructureProducerCompatible() bool { return runtime.Version() == "go1.27.3" }

// KnowledgeStructureProducerDigest binds every frozen budget and explicit alias.
func KnowledgeStructureProducerDigest(aliases []KnowledgeStructureAlias) (string, error) {
	var value map[string]any
	decoder := json.NewDecoder(bytes.NewReader(KnowledgeStructureProducerJSON))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return "", err
	}
	ordered := slices.Clone(aliases)
	if ordered == nil {
		ordered = []KnowledgeStructureAlias{}
	}
	slices.SortFunc(ordered, func(a, b KnowledgeStructureAlias) int { return bytes.Compare([]byte(a.Prefix), []byte(b.Prefix)) })
	value["aliases"] = ordered
	return KnowledgeLifecycleDigest(value)
}
