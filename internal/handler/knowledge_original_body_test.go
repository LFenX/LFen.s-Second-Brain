package handler

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type originalBodyConnectionKey struct{}

func TestKnowledgeOriginalBodyEOFLeavesConnectionUsable(t *testing.T) {
	for _, chunked := range []bool{false, true} {
		t.Run(fmt.Sprintf("chunked=%t", chunked), func(t *testing.T) {
			var connections atomic.Int64
			contexts := make(chan context.Context, 1)
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Test-Connection", fmt.Sprint(r.Context().Value(originalBodyConnectionKey{})))
				if r.Method != http.MethodPut {
					if r.Context().Err() != nil {
						w.WriteHeader(http.StatusUnauthorized)
					} else {
						w.WriteHeader(http.StatusCreated)
					}
					return
				}
				controller := http.NewResponseController(w)
				if err := controller.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				defer controller.SetReadDeadline(time.Time{})
				body := &originalRequestBody{ReadCloser: r.Body, controller: controller}
				if _, err := io.Copy(io.Discard, body); err != nil {
					t.Error(err)
				}
				if err := body.Close(); err != nil {
					t.Error(err)
				}
				// The receiver releases its database slot after closing the body.
				time.Sleep(10 * time.Millisecond)
				contexts <- r.Context()
				w.WriteHeader(http.StatusOK)
			}))
			server.Config.ConnContext = func(ctx context.Context, _ net.Conn) context.Context {
				return context.WithValue(ctx, originalBodyConnectionKey{}, connections.Add(1))
			}
			server.Start()
			defer server.Close()
			transport := &http.Transport{MaxConnsPerHost: 1, MaxIdleConnsPerHost: 1}
			defer transport.CloseIdleConnections()
			client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
			var input io.Reader = bytes.NewBufferString("synthetic upload")
			if chunked {
				input = io.NopCloser(input)
			}
			put, err := http.NewRequest(http.MethodPut, server.URL, input)
			if err != nil {
				t.Fatal(err)
			}
			response, err := client.Do(put)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			connection := response.Header.Get("X-Test-Connection")
			if response.StatusCode != http.StatusOK {
				t.Fatal("upload status", response.StatusCode)
			}
			reused := false
			trace := &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) { reused = info.Reused }}
			request, err := http.NewRequestWithContext(httptrace.WithClientTrace(t.Context(), trace), http.MethodPost, server.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			response, err = client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			if !reused || response.Header.Get("X-Test-Connection") != connection || connections.Load() != 1 {
				t.Fatal("test did not reuse the upload connection")
			}
			if response.StatusCode != http.StatusCreated {
				t.Errorf("request following complete upload: status=%d want=201", response.StatusCode)
			}
			// Request contexts cancel on handler return; inspect the next request's
			// status above, rather than reading this context after that return.
			<-contexts
		})
	}
}

func TestKnowledgeOriginalBodyCloseInterruptsIncompleteRead(t *testing.T) {
	started := make(chan struct{})
	interrupt := make(chan struct{})
	closed := make(chan struct{})
	readResult := make(chan error, 1)
	var release sync.Once
	defer release.Do(func() { close(interrupt) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		controller := http.NewResponseController(w)
		if err := controller.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
			readResult <- err
			return
		}
		defer controller.SetReadDeadline(time.Time{})
		body := &originalRequestBody{ReadCloser: r.Body, controller: controller}
		go func() {
			<-interrupt
			_ = body.Close()
			close(closed)
		}()
		close(started)
		_, err := io.Copy(io.Discard, body)
		readResult <- err
		<-closed
		r.Close = true
		w.Header().Set("Connection", "close")
		w.WriteHeader(http.StatusRequestTimeout)
	}))
	defer server.Close()
	connection, err := net.DialTimeout("tcp", strings.TrimPrefix(server.URL, "http://"), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if _, err := fmt.Fprintf(connection, "PUT / HTTP/1.1\r\nHost: synthetic.test\r\nContent-Length: 1024\r\n\r\nabc"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not start the body read")
	}
	release.Do(func() { close(interrupt) })
	select {
	case err := <-readResult:
		if err == nil {
			t.Error("incomplete input was accepted as a complete body")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not interrupt the body read")
	}
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not return")
	}
}
