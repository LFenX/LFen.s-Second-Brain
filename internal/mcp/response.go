package mcp

import (
	"bytes"
	"net/http"
)

const knowledgeMaxResponseBytes = 1 << 20

type knowledgeResponseBuffer struct {
	header   http.Header
	status   int
	body     bytes.Buffer
	overflow bool
}

func (b *knowledgeResponseBuffer) Header() http.Header { return b.header }
func (b *knowledgeResponseBuffer) WriteHeader(status int) {
	if b.status == 0 {
		b.status = status
	}
}
func (b *knowledgeResponseBuffer) Write(data []byte) (int, error) {
	if b.status == 0 {
		b.status = http.StatusOK
	}
	if b.overflow || len(data) > knowledgeMaxResponseBytes-b.body.Len() {
		b.overflow = true
		return len(data), nil
	}
	return b.body.Write(data)
}

// SDK flushes remain private until the complete response passes its gate.
func (*knowledgeResponseBuffer) Flush() {}
