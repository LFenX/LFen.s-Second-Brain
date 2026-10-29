package dataset

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// The first Close interrupts Read immediately but takes longer to finish.
// A second Close returns immediately, exposing callers that fail to join the
// cancellation watcher before returning their body to the HTTP adapter.
type knowledgeUploadDelayedCloseBody struct {
	readStarted   chan struct{}
	readReleased  chan struct{}
	closeStarted  chan struct{}
	closeReleased chan struct{}
	closeFinished chan struct{}
	readOnce      sync.Once
	releaseOnce   sync.Once
	closes        atomic.Int64
}

func (b *knowledgeUploadDelayedCloseBody) Read([]byte) (int, error) {
	b.readOnce.Do(func() { close(b.readStarted) })
	<-b.readReleased
	return 0, io.ErrUnexpectedEOF
}

func (b *knowledgeUploadDelayedCloseBody) Close() error {
	if b.closes.Add(1) == 1 {
		close(b.closeStarted)
		close(b.readReleased)
		<-b.closeReleased
		close(b.closeFinished)
	}
	return nil
}

func (b *knowledgeUploadDelayedCloseBody) release() {
	b.releaseOnce.Do(func() { close(b.closeReleased) })
}

func TestKnowledgeUploadReceiveWaitsForBodyClose(t *testing.T) {
	for _, family := range []string{"original", "web"} {
		for _, deadline := range []bool{false, true} {
			name := family + "/cancel"
			if deadline {
				name = family + "/deadline"
			}
			t.Run(name, func(t *testing.T) {
				body := &knowledgeUploadDelayedCloseBody{
					readStarted: make(chan struct{}), readReleased: make(chan struct{}),
					closeStarted: make(chan struct{}), closeReleased: make(chan struct{}), closeFinished: make(chan struct{}),
				}
				defer body.release()
				var receive func(context.Context) error
				var checkState func()
				if family == "original" {
					s, reg, _ := originalFlowFixture(t)
					v, _, err := s.CreateKnowledgeOriginalUpload(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, originalTestRequest("body:lifetime"))
					if err != nil {
						t.Fatal(err)
					}
					receive = func(ctx context.Context) error {
						_, err := s.ReceiveKnowledgeOriginal(ctx, knowledgeTestOwner, reg.DomainID, reg.RegistrationID, v.UploadID, body, -1, "", "", "application/octet-stream")
						return err
					}
					checkState = func() {
						view, err := s.GetKnowledgeOriginalUpload(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, v.UploadID, "")
						if err != nil || view.State != "interrupted" {
							t.Fatal("upload state", view, err)
						}
						var slots int64
						if err := dao.DB.Model(&entity.KnowledgeOriginalReceiveSlot{}).Where("upload_id = ?", v.UploadID).Count(&slots).Error; err != nil || slots != 0 {
							t.Fatal("receive slot was not released", slots, err)
						}
					}
				} else {
					s, reg, _ := webFlowFixture(t)
					v, _, err := s.CreateKnowledgeWebCapture(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, webLocalTestRequest(reg, "body:lifetime"))
					if err != nil {
						t.Fatal(err)
					}
					receive = func(ctx context.Context) error {
						_, err := s.ReceiveKnowledgeWebCapture(ctx, knowledgeTestOwner, reg.DomainID, reg.RegistrationID, v.CaptureID, body, -1, "", "", "application/octet-stream")
						return err
					}
					checkState = func() {
						view, err := s.GetKnowledgeWebCapture(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, v.CaptureID, "")
						if err != nil || view.State != "interrupted" {
							t.Fatal("capture state", view, err)
						}
						var slots int64
						if err := dao.DB.Model(&entity.KnowledgeWebSlot{}).Where("capture_id = ?", v.CaptureID).Count(&slots).Error; err != nil || slots != 0 {
							t.Fatal("capture slot was not released", slots, err)
						}
					}
				}
				ctx, cancel := context.WithCancel(t.Context())
				if deadline {
					cancel()
					ctx, cancel = context.WithTimeout(t.Context(), 500*time.Millisecond)
				}
				defer cancel()
				result := make(chan error, 1)
				go func() { result <- receive(ctx) }()
				select {
				case <-body.readStarted:
				case <-time.After(3 * time.Second):
					t.Fatal("body read did not start")
				}
				if !deadline {
					cancel()
				}
				select {
				case <-body.closeStarted:
				case <-time.After(3 * time.Second):
					t.Fatal("cancellation did not close the body")
				}
				var receiveErr error
				early := false
				select {
				case receiveErr = <-result:
					early = true
					t.Error("receive returned while its cancellation Close was still running")
				case <-time.After(100 * time.Millisecond):
				}
				body.release()
				if !early {
					select {
					case receiveErr = <-result:
					case <-time.After(3 * time.Second):
						t.Fatal("receive did not finish after Close returned")
					}
				}
				select {
				case <-body.closeFinished:
				case <-time.After(time.Second):
					t.Fatal("Close did not finish")
				}
				if count := body.closes.Load(); count != 1 {
					t.Errorf("body Close called %d times; want exactly once", count)
				}
				status, code := 409, "upload_interrupted"
				if family == "web" {
					code = "web_capture_interrupted"
				}
				if deadline {
					code = "upload_timeout"
					if family == "web" {
						status, code = 504, "web_upload_timeout"
					}
				}
				assertOriginalCode(t, receiveErr, status, code)
				checkState()
			})
		}
	}
}
