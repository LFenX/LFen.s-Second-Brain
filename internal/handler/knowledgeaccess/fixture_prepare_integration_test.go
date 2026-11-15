//go:build integration && linux

package knowledgeaccess

import (
	"os"
	"ragflow/internal/entity"
	"testing"
	"time"
)

func TestKnowledgeAccessFixturePreparation(t *testing.T) {
	scope := readFixtureLease(t)
	if fixtureCheckMaterials() != nil {
		t.Fatal("fixture_materials_refused")
	}
	if len(fixturePreparationGaps(scope, os.Getenv("LFEN_ACCESS_FIXTURE_SOURCE_SET_SHA256"), time.Now())) != 0 {
		t.Fatal("fixture_not_ready_owning_recipe")
	}
	root, err := repositoryRoot()
	if err != nil {
		t.Fatal("fixture_contract_root_refused")
	}
	contracts, err := loadContracts(root)
	if err != nil {
		t.Fatal("fixture_contracts_refused")
	}
	client := newFixtureHTTP(scope)
	defer client.close()
	actors, err := client.prepareAccounts()
	if err != nil {
		t.Fatal("fixture_native_accounts_refused")
	}
	domains, err := client.prepareDomains(actors)
	if err != nil {
		t.Fatal("fixture_native_domains_refused")
	}
	domain := domains["owner"]
	c1, err := client.prepareC1(domain)
	if err != nil {
		t.Fatal("fixture_c1_refused")
	}
	r1, err := client.prepareR1(domain, actors.Public["owner"].UserID)
	if err != nil {
		t.Fatal("fixture_r1_refused")
	}
	docx, err := client.prepareDOCX(domain)
	if err != nil {
		t.Fatal("fixture_docx_refused")
	}
	a, err := client.prepareParsed(domain, "synthetic-A.csv")
	if err != nil {
		t.Fatal("fixture_a_refused")
	}
	denied, err := client.prepareParsed(domain, "synthetic-B.csv")
	if err != nil {
		t.Fatal("fixture_b_refused")
	}
	ar2, err := client.snapshotAR2(domain, actors.Public["owner"].UserID, a)
	if err != nil {
		t.Fatal("fixture_a_r2_refused")
	}
	c1id := c1.RevisionID
	selection := entity.KnowledgeContentLexicalSelection{Kind: "fixed_representations", Targets: []entity.KnowledgeContentTarget{
		{Source: c1, Representation: entity.KnowledgeContentRepresentationRef{Kind: "source_revision", ID: &c1id}}, r1, docx, a, denied}}
	names := map[string]string{c1.SourceID: "long-source.txt", r1.Source.SourceID: "guide.md", docx.Source.SourceID: "empty-cell.docx", a.Source.SourceID: "synthetic-A.csv", denied.Source.SourceID: "synthetic-B.csv"}
	fixture, err := client.finishFixture(actors, domains, selection, ar2, names, contracts)
	if err != nil {
		t.Fatal("fixture_owner_bridge_or_oracle_refused")
	}
	if client.handoffFixture(fixture, actors) != nil {
		t.Fatal("fixture_ram_handoff_refused")
	}
}
