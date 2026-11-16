package config

import (
	"strings"
	"testing"
)

func TestKnowledgeAccessMCPAudience(t *testing.T) {
	for _, origin := range []string{"https://knowledge.example.invalid", "http://127.0.0.1:29398", "http://localhost:29398", "http://[::1]:29398"} {
		value := origin + KnowledgeAccessMCPPath
		actual, err := knowledgeAccessMCPAudience(value)
		if err != nil || actual != value {
			t.Fatalf("canonical audience rejected")
		}
	}
	for _, value := range []string{
		"", " " + "https://knowledge.example.invalid" + KnowledgeAccessMCPPath,
		"http://knowledge.example.invalid" + KnowledgeAccessMCPPath,
		"http://0.0.0.0:29398" + KnowledgeAccessMCPPath,
		"http://localhost.attacker.invalid" + KnowledgeAccessMCPPath,
		"https://user:blocked-secret@knowledge.example.invalid" + KnowledgeAccessMCPPath,
		"https://knowledge.example.invalid" + KnowledgeAccessMCPPath + "?token=blocked-secret",
		"https://knowledge.example.invalid" + KnowledgeAccessMCPPath + "?",
		"https://knowledge.example.invalid" + KnowledgeAccessMCPPath + "#fragment",
		"https://knowledge.example.invalid" + KnowledgeAccessMCPPath + "/",
		"https://knowledge.example.invalid/api/v1/knowledge/access/%6dcp",
		"HTTPS://knowledge.example.invalid" + KnowledgeAccessMCPPath,
		"https://Knowledge.example.invalid" + KnowledgeAccessMCPPath,
		"http://localhost:0" + KnowledgeAccessMCPPath,
		"http://localhost:65536" + KnowledgeAccessMCPPath,
		"http://localhost:" + KnowledgeAccessMCPPath,
		"https://knowledge.example.invalid" + KnowledgeAccessMCPPath + "\n",
		strings.Repeat("x", 2049),
	} {
		actual, err := knowledgeAccessMCPAudience(value)
		if err == nil || actual != "" || strings.Contains(err.Error(), "blocked-secret") {
			t.Fatalf("invalid audience accepted or disclosed")
		}
	}
}

func TestKnowledgeAccessMCPAudienceRequiresExplicitConfiguration(t *testing.T) {
	t.Setenv("KNOWLEDGE_ACCESS_MCP_AUDIENCE", "")
	if _, err := KnowledgeAccessMCPAudience(); err == nil {
		t.Fatal("missing explicit configuration was accepted")
	}
	want := "http://localhost:29398" + KnowledgeAccessMCPPath
	t.Setenv("KNOWLEDGE_ACCESS_MCP_AUDIENCE", want)
	if actual, err := KnowledgeAccessMCPAudience(); err != nil || actual != want {
		t.Fatal("explicit configuration not preserved exactly")
	}
}
