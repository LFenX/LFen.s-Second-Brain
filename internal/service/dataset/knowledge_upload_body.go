package dataset

import (
	"context"
	"io"
)

// watchKnowledgeUploadBody closes the body once on cancellation or completion.
// The returned function joins that closer before the HTTP adapter can reset its
// connection deadline or reuse its response writer for another request.
func watchKnowledgeUploadBody(ctx context.Context, body io.Closer) func() {
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case <-ctx.Done():
		case <-stop:
		}
		_ = body.Close()
	}()
	return func() {
		close(stop)
		<-done
	}
}
