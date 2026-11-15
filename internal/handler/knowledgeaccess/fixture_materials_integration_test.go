//go:build integration && linux

package knowledgeaccess

import (
	"embed"
	"fmt"
	"unicode/utf8"
)

const fixtureMaterialManifestSHA = "911ec2806fa30e46be56b0ba753e853c3ddd9d03cba95d7e33ba4b4fbe57d48e"

//go:embed testdata/access-fixture/*
var fixtureMaterialFS embed.FS

type fixtureMaterialPin struct {
	bytes int
	sha   string
}

var fixtureMaterialPins = map[string]fixtureMaterialPin{
	"long-source.txt":    {bytes: 9600, sha: "c18397c612a08773e373ffc2a56544363c1ab9430206e794a2766056cd12c6f7"},
	"guide.md":           {bytes: 52, sha: "7da83199ec19ea7e41c70c805e18818984e2d4bbb3df4c79de3b71508e3436ce"},
	"empty-cell.docx":    {bytes: 1258, sha: "33f65648aeaa02abd18783ec2834935914e34c17a9f6fe82bcc638885a3cf745"},
	"synthetic-A.csv":    {bytes: 36, sha: "f04a56bbb3b93dbf8cb01cb26076dcae7983dd860d7d0fc2ec1a909ed3116de5"},
	"synthetic-B.csv":    {bytes: 44, sha: "fc64a9cc671db5e0f4bec43b1466a9b13b1bdf7cda82b18ad1a0e026b163f941"},
	"synthetic-A-r2.csv": {bytes: 49, sha: "a4bba31717c916378247035d413a41454b79c4aabec4cb996d65668a0865bac1"},
}

func fixtureMaterial(name string) ([]byte, error) {
	pin, ok := fixtureMaterialPins[name]
	if !ok {
		return nil, errFixture
	}
	raw, err := fixtureMaterialFS.ReadFile("testdata/access-fixture/" + name)
	if err != nil || len(raw) != pin.bytes || digest(raw) != pin.sha {
		return nil, errFixture
	}
	max := 8192
	if name == "long-source.txt" {
		max = 16384
		if len(raw) <= 8192 {
			return nil, errFixture
		}
	}
	if len(raw) > max || (name != "empty-cell.docx" && !utf8.Valid(raw)) {
		return nil, errFixture
	}
	return raw, nil
}
func fixtureCheckMaterials() error {
	total := 0
	for name := range fixtureMaterialPins {
		raw, err := fixtureMaterial(name)
		if err != nil {
			return err
		}
		total += len(raw)
	}
	if total > 65536 {
		return fmt.Errorf("fixture_material_budget_refused")
	}
	return nil
}
