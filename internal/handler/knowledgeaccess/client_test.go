package knowledgeaccess

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxRequestBytes  = 32 << 10
	maxResponseBytes = 1 << 20
	maxRequests      = 120
	maxTotalBytes    = 16 << 20
	runDuration      = 10 * time.Minute
)

var (
	errScope    = errors.New("acceptance_scope_refused")
	errRoute    = errors.New("acceptance_route_refused")
	errBudget   = errors.New("acceptance_budget_exhausted")
	errExchange = errors.New("acceptance_exchange_failed")
)

type runScope struct {
	id           string
	origin       string
	notBefore    time.Time
	expires      time.Time
	synthetic    bool
	s1Mutations  bool
	nativeModels bool
}

// All credentials remain in memory. Callers never receive transport errors,
// request headers or raw server diagnostics in a test failure message.
type accessClient struct {
	origin     *url.URL
	scope      runScope
	auth       map[string]string
	routes     map[string]string
	http       *http.Client
	mu         sync.Mutex
	count      int
	total      int
	stopped    error
	operations map[string]int
}

type response struct {
	status int
	header http.Header
	body   []byte
}

func localOrigin(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Path != "" || u.RawPath != "" || u.Opaque != "" {
		return nil, errScope
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, errScope
	}
	ip := net.ParseIP(u.Hostname())
	port, portErr := strconv.Atoi(u.Port())
	if ip == nil || !ip.IsLoopback() || portErr != nil || port < 1 || port > 65535 {
		return nil, errScope
	}
	return u, nil
}

func s1Paths() map[string]string {
	return map[string]string{
		"setPolicy":      "/api/v1/knowledge/access/admin/domains/policy/set",
		"setModule":      "/api/v1/knowledge/access/admin/modules/set",
		"getModuleAdmin": "/api/v1/knowledge/access/admin/modules/get",
		"listModules":    "/api/v1/knowledge/access/modules/list",
		"createGrant":    "/api/v1/knowledge/access/admin/grants/create",
		"revokeGrant":    "/api/v1/knowledge/access/admin/grants/revoke",
		"inspect":        "/api/v1/knowledge/access/inspect",
		"search":         "/api/v1/knowledge/access/search",
		"read":           "/api/v1/knowledge/access/read",
	}
}

func allowedOperation(op string) bool {
	switch op {
	case "setPolicy", "setModule", "getModuleAdmin", "listModules", "createGrant", "revokeGrant", "inspect", "search", "read":
		return true
	default:
		return false
	}
}

func newAccessClient(scope runScope, origin string, auth map[string]string, routes map[string]string, transport http.RoundTripper) (*accessClient, error) {
	u, err := localOrigin(origin)
	now := time.Now()
	if err != nil || scope.id == "" || !scope.synthetic || (!scope.s1Mutations && !scope.nativeModels) || scope.origin != origin || scope.notBefore.IsZero() || now.Before(scope.notBefore) || !scope.expires.After(now) || scope.expires.Sub(scope.notBefore) > runDuration {
		return nil, errScope
	}
	if len(routes) != 9 {
		return nil, errRoute
	}
	fixedRoutes := make(map[string]string, len(routes))
	for op, route := range routes {
		if !allowedOperation(op) || route != s1Paths()[op] {
			return nil, errRoute
		}
		fixedRoutes[op] = route
	}
	credentials := make(map[string]string, len(auth))
	for actor, header := range auth {
		if actor != "owner" && actor != "recipient" && actor != "foreign" {
			return nil, errScope
		}
		if header == "" || strings.TrimSpace(header) != header || len(header) > 4096 {
			return nil, errScope
		}
		for _, value := range []byte(header) {
			if value < 0x20 || value == 0x7f {
				return nil, errScope
			}
		}
		for _, previous := range credentials {
			if previous == header {
				return nil, errScope
			}
		}
		credentials[actor] = header
	}
	if len(credentials) != 3 {
		return nil, errScope
	}
	if transport == nil {
		dialer := &net.Dialer{Timeout: 3 * time.Second}
		transport = &http.Transport{
			Proxy: nil,
			DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				if address != u.Host {
					return nil, errRoute
				}
				return dialer.DialContext(ctx, network, address)
			},
			DisableKeepAlives:      true,
			MaxConnsPerHost:        1,
			MaxResponseHeaderBytes: 16 << 10,
			ResponseHeaderTimeout:  5 * time.Second,
			TLSHandshakeTimeout:    3 * time.Second,
		}
	}
	return &accessClient{
		origin: u, scope: scope, auth: credentials, routes: fixedRoutes, operations: make(map[string]int),
		http: &http.Client{
			Transport: transport, Timeout: 15 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

func (c *accessClient) post(ctx context.Context, actor, operation, domain string, body []byte) (response, error) {
	if !c.scope.s1Mutations || time.Now().Before(c.scope.notBefore) || !time.Now().Before(c.scope.expires) || !allowedOperation(operation) || c.auth[actor] == "" {
		return response{}, errScope
	}
	if decoded, err := hex.DecodeString(domain); err != nil || len(decoded) != 16 || strings.ToLower(domain) != domain {
		return response{}, errRoute
	}
	return c.exchange(ctx, actor, operation, http.MethodPost, c.routes[operation], body)
}

func (c *accessClient) patchModels(ctx context.Context, actor string, body []byte) (response, error) {
	if !c.scope.nativeModels {
		return response{}, errScope
	}
	return c.exchange(ctx, actor, "nativeModels", http.MethodPatch, "/api/v1/users/me/models", body)
}

func (c *accessClient) secretMarkers() []string {
	values := []string{}
	for _, header := range c.auth {
		values = append(values, header)
		parts := strings.Fields(header)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			values = append(values, parts[1])
		}
	}
	return values
}

func (c *accessClient) exchange(ctx context.Context, actor, operation, method, route string, body []byte) (response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped != nil {
		return response{}, c.stopped
	}
	if time.Now().Before(c.scope.notBefore) || !time.Now().Before(c.scope.expires) || c.auth[actor] == "" {
		c.stopped = errScope
		return response{}, errScope
	}
	if operation != "nativeModels" && (method != http.MethodPost || route != s1Paths()[operation] || route == "") {
		return response{}, errRoute
	}
	if operation != "nativeModels" && !c.scope.s1Mutations {
		return response{}, errScope
	}
	if operation == "nativeModels" && (!c.scope.nativeModels || method != http.MethodPatch || route != "/api/v1/users/me/models") {
		return response{}, errRoute
	}
	if len(body) > maxRequestBytes || c.count >= maxRequests || c.total+len(body) > maxTotalBytes {
		c.stopped = errBudget
		return response{}, errBudget
	}
	c.count++
	c.total += len(body)
	c.operations[operation]++
	u := *c.origin
	u.Path = route
	requestContext, cancel := context.WithDeadline(ctx, c.scope.expires)
	defer cancel()
	req, err := http.NewRequestWithContext(requestContext, method, u.String(), bytes.NewReader(body))
	if err != nil {
		return response{}, errRoute
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", c.auth[actor])
	r, err := c.http.Do(req)
	if err != nil {
		c.stopped = errExchange
		return response{}, errExchange
	}
	defer r.Body.Close()
	remaining := maxTotalBytes - c.total
	limit := min(maxResponseBytes, remaining)
	raw, err := io.ReadAll(io.LimitReader(r.Body, int64(limit+1)))
	c.total += len(raw)
	if err != nil || len(raw) > limit {
		c.stopped = errBudget
		return response{}, errBudget
	}
	if !time.Now().Before(c.scope.expires) {
		c.stopped = errScope
		return response{}, errScope
	}
	return response{status: r.StatusCode, header: r.Header.Clone(), body: raw}, nil
}
