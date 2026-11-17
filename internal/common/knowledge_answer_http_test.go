package common

import (
	"net/http"
	"testing"
)

func TestPrivateProviderClientNeverFollowsRedirectsOrLogsPayloads(t *testing.T) {
	client := PrivateProviderHTTPClient()
	if client.CheckRedirect(&http.Request{}, nil) != http.ErrUseLastResponse {
		t.Fatal("redirect allowed")
	}
	if _, ok := client.Transport.(*strictSSRFTransport); !ok {
		t.Fatal("private client lost SSRF guard or gained payload logging")
	}
	if client == GetSSRFHTTPClient() || client == GetSchemeSafeHTTPClient() {
		t.Fatal("mutated a shared provider client")
	}
}
