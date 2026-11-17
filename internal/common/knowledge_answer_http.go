package common

import (
	"net/http"
	"sync"
	"time"
)

var privateProviderClient = sync.OnceValue(func() *http.Client {
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment}
	if standard, ok := http.DefaultTransport.(*http.Transport); ok {
		transport = standard.Clone()
	}
	transport.MaxIdleConns = 20
	transport.MaxIdleConnsPerHost = 10
	transport.IdleConnTimeout = 90 * time.Second
	transport.ResponseHeaderTimeout = 60 * time.Second
	return &http.Client{
		Transport:     &strictSSRFTransport{base: transport},
		Timeout:       65 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
})

// PrivateProviderHTTPClient retains the native SSRF guard, refuses redirects
// and never installs payload logging, including when LLM_DEBUG is enabled.
func PrivateProviderHTTPClient() *http.Client { return privateProviderClient() }
