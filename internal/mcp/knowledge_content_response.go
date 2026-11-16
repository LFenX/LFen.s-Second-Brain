package mcp

import (
	"net/http"
	"strconv"
)

func contentKnowledgeBoundedHTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		buffer := &knowledgeResponseBuffer{header: make(http.Header)}
		next.ServeHTTP(buffer, r)
		if buffer.overflow {
			// HTTP 413 is a transport capacity refusal, not partial tool success.
			// The bounded null-id error also handles an oversized JSON-RPC id.
			body := []byte(`{"jsonrpc":"2.0","id":null,"error":{"code":-32000,"message":"Knowledge content response capacity exceeded","data":{"code":"capacity_exceeded"}}}`)
			w.Header().Del("Transfer-Encoding")
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			_, _ = w.Write(body)
			return
		}
		for key, values := range buffer.header {
			w.Header()[key] = append([]string(nil), values...)
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Del("Transfer-Encoding")
		w.Header().Set("Content-Length", strconv.Itoa(buffer.body.Len()))
		status := buffer.status
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		_, _ = w.Write(buffer.body.Bytes())
	})
}
