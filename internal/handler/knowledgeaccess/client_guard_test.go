package knowledgeaccess

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func guardRoutes() map[string]string {
	return s1Paths()
}

func guardAuth() map[string]string {
	return map[string]string{"owner": "unit-owner", "recipient": "unit-recipient", "foreign": "unit-foreign"}
}

func guardScope(origin string) runScope {
	return runScope{id: "unit-scope", origin: origin, notBefore: time.Now().Add(-time.Second), expires: time.Now().Add(time.Minute), synthetic: true, s1Mutations: true}
}

func TestAccessClientRefusesInvalidScopeBeforeTransport(t *testing.T) {
	const origin = "http://127.0.0.1:19001"
	calls := 0
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("unit-sensitive-transport-diagnostic")
	})
	for _, raw := range []string{"", "https://example.invalid:443", "http://127.0.0.1:0", "http://127.0.0.1:70000", "http://127.0.0.1:19001/path", "http://127.0.0.1:19001?token=unit", "http://unit@127.0.0.1:19001"} {
		if _, err := newAccessClient(guardScope(raw), raw, guardAuth(), guardRoutes(), transport); err == nil {
			t.Fatal("invalid origin was accepted")
		}
	}
	for _, scope := range []runScope{{}, {id: "unit", origin: origin, expires: time.Now().Add(time.Minute)}, {id: "unit", origin: origin, synthetic: true, expires: time.Now().Add(-time.Second)}, {id: "unit", origin: origin, synthetic: true, expires: time.Now().Add(time.Hour)}, guardScope("http://127.0.0.1:19002")} {
		if _, err := newAccessClient(scope, origin, guardAuth(), guardRoutes(), transport); err == nil {
			t.Fatal("invalid scope was accepted")
		}
	}
	if calls != 0 {
		t.Fatal("scope validation reached the transport")
	}
}

func TestAccessClientRejectsIdentityReuseAndExtraRoute(t *testing.T) {
	const origin = "http://127.0.0.1:19001"
	auth := guardAuth()
	auth["recipient"] = auth["owner"]
	if _, err := newAccessClient(guardScope(origin), origin, auth, guardRoutes(), nil); err == nil {
		t.Fatal("recipient reused an owner session")
	}
	routes := guardRoutes()
	routes["createCredential"] = "/api/v1/knowledge/access/mcp"
	if _, err := newAccessClient(guardScope(origin), origin, guardAuth(), routes, nil); err == nil {
		t.Fatal("an operation outside S1 was admitted")
	}
}

func TestAccessClientBudgetAndDiagnostics(t *testing.T) {
	const origin = "http://127.0.0.1:19001"
	calls := 0
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("unit-sensitive-transport-diagnostic")
	})
	client, err := newAccessClient(guardScope(origin), origin, guardAuth(), guardRoutes(), transport)
	if err != nil {
		t.Fatal("unit client initialization failed")
	}
	domain := strings.Repeat("a", 32)
	if _, err := client.post(context.Background(), "recipient", "read", domain, make([]byte, maxRequestBytes+1)); err != errBudget || calls != 0 {
		t.Fatal("oversize request reached the transport")
	}
	if _, err := client.post(context.Background(), "recipient", "createCredential", domain, nil); err == nil || calls != 0 {
		t.Fatal("non-S1 operation reached the transport")
	}
	if _, err := client.post(context.Background(), "recipient", "read", domain, nil); err != errBudget || calls != 0 {
		t.Fatal("budget failure did not permanently stop this run")
	}
	client, err = newAccessClient(guardScope(origin), origin, guardAuth(), guardRoutes(), transport)
	if err != nil {
		t.Fatal("unit client initialization failed")
	}
	if _, err := client.post(context.Background(), "recipient", "read", domain, []byte("{}")); err != errExchange || calls != 1 || strings.Contains(err.Error(), "sensitive") {
		t.Fatal("transport diagnostic was exposed")
	}
	client, err = newAccessClient(guardScope(origin), origin, guardAuth(), guardRoutes(), transport)
	if err != nil {
		t.Fatal("unit client initialization failed")
	}
	client.count = maxRequests
	if _, err := client.post(context.Background(), "recipient", "read", domain, nil); err != errBudget || calls != 1 {
		t.Fatal("request count budget was exceeded")
	}
}

func TestAccessClientDoesNotFollowRedirect(t *testing.T) {
	forwarded := 0
	sink := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { forwarded++ }))
	defer sink.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, sink.URL, http.StatusTemporaryRedirect)
	}))
	defer origin.Close()
	client, err := newAccessClient(guardScope(origin.URL), origin.URL, guardAuth(), guardRoutes(), nil)
	if err != nil {
		t.Fatal("unit client initialization failed")
	}
	result, err := client.post(context.Background(), "recipient", "read", strings.Repeat("a", 32), []byte("{}"))
	if err != nil || result.status != http.StatusTemporaryRedirect || forwarded != 0 {
		t.Fatal("redirect crossed the approved origin")
	}
}

func TestAccessClientBoundsResponseAndSnapshotsRoutes(t *testing.T) {
	const origin = "http://127.0.0.1:19001"
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(strings.Repeat("x", maxResponseBytes+1)))}, nil
	})
	routes := guardRoutes()
	client, err := newAccessClient(guardScope(origin), origin, guardAuth(), routes, transport)
	if err != nil {
		t.Fatal("unit client initialization failed")
	}
	routes["read"] = "/forbidden"
	if client.routes["read"] != "/api/v1/knowledge/access/read" {
		t.Fatal("caller changed the route snapshot")
	}
	if result, err := client.post(context.Background(), "recipient", "read", strings.Repeat("a", 32), nil); err != errBudget || len(result.body) != 0 {
		t.Fatal("oversize response escaped its byte bound")
	}
	if client.total < maxResponseBytes+1 || client.stopped != errBudget {
		t.Fatal("failed response bytes were not charged and stopped")
	}
}

func TestAccessClientRejectsSamePrefixRouteSubstitution(t *testing.T) {
	const origin = "http://127.0.0.1:19001"
	for op := range s1Paths() {
		routes := guardRoutes()
		routes[op] = "/api/v1/knowledge/access/admin/departments/set"
		if _, err := newAccessClient(guardScope(origin), origin, guardAuth(), routes, nil); err != errRoute {
			t.Fatal("a substituted approved-prefix route was accepted")
		}
	}
}

func TestAccessClientExpiredWhileWaitingCannotSend(t *testing.T) {
	const origin = "http://127.0.0.1:19001"
	calls := 0
	c, err := newAccessClient(guardScope(origin), origin, guardAuth(), guardRoutes(), roundTripFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, errors.New("not reached") }))
	if err != nil {
		t.Fatal("unit client initialization failed")
	}
	c.scope.expires = time.Now().Add(-time.Second)
	if _, err := c.post(context.Background(), "owner", "inspect", strings.Repeat("a", 32), nil); err != errScope || calls != 0 {
		t.Fatal("expired run reached transport")
	}
}

func TestAccessClientNativePatchIsSeparatelyBound(t *testing.T) {
	const origin = "http://127.0.0.1:19001"
	calls := 0
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != http.MethodPatch || r.URL.Path != "/api/v1/users/me/models" {
			t.Fatal("native patch escaped its exact surface")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	})
	c, err := newAccessClient(guardScope(origin), origin, guardAuth(), guardRoutes(), transport)
	if err != nil {
		t.Fatal("unit client initialization failed")
	}
	if _, err := c.patchModels(context.Background(), "owner", []byte(`{}`)); err != errScope || calls != 0 {
		t.Fatal("native patch ran without separate scope")
	}
	scope := guardScope(origin)
	scope.s1Mutations = false
	scope.nativeModels = true
	c, err = newAccessClient(scope, origin, guardAuth(), guardRoutes(), transport)
	if err != nil {
		t.Fatal("unit client initialization failed")
	}
	for op, route := range s1Paths() {
		if _, err = c.post(context.Background(), "owner", op, strings.Repeat("a", 32), []byte(`{}`)); err != errScope || calls != 0 {
			t.Fatal("native-only scope admitted an S1 operation")
		}
		if _, err = c.exchange(context.Background(), "owner", op, http.MethodPost, route, []byte(`{}`)); err != errScope || calls != 0 {
			t.Fatal("native-only exchange admitted an S1 operation")
		}
	}
	if _, err = c.patchModels(context.Background(), "owner", []byte(`{}`)); err != nil || calls != 1 {
		t.Fatal("bound native patch failed")
	}
}
