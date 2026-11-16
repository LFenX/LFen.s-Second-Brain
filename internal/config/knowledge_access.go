package config

import (
	"errors"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const KnowledgeAccessMCPPath = "/api/v1/knowledge/access/mcp"

// KnowledgeAccessMCPAudience returns the explicitly configured service identity.
// External delivery requires HTTPS; HTTP is limited to loopback development.
func KnowledgeAccessMCPAudience() (string, error) {
	return knowledgeAccessMCPAudience(os.Getenv("KNOWLEDGE_ACCESS_MCP_AUDIENCE"))
}

func knowledgeAccessMCPAudience(value string) (string, error) {
	fail := func() (string, error) { return "", errors.New("knowledge access MCP audience is unavailable") }
	if value == "" || len(value) > 2048 {
		return fail()
	}
	for _, c := range []byte(value) {
		if c < 33 || c > 126 {
			return fail()
		}
	}
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || u.Host != strings.ToLower(u.Host) || u.User != nil || u.Opaque != "" || u.Path != KnowledgeAccessMCPPath || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" || u.String() != value || strings.HasSuffix(u.Host, ":") {
		return fail()
	}
	if port := u.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return fail()
		}
	}
	loopback := u.Hostname() == "localhost"
	if address := net.ParseIP(u.Hostname()); address != nil {
		loopback = address.IsLoopback()
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback) {
		return fail()
	}
	return value, nil
}
