package handler

import (
	"io"
	"mime"
	"net/http"

	"github.com/gin-gonic/gin"
	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service/dataset"
)

func answerHTTPError(c *gin.Context, operation, requestID string, status int, code string) {
	status, raw := dataset.AnswerErrorResponse(operation, requestID, &dataset.KnowledgeAnswerError{Status: status, Code: code})
	c.Data(status, "application/json; charset=utf-8", raw)
}

func KnowledgeAnswerAuth(auth gin.HandlerFunc, operation string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Header("X-Knowledge-Contract-Version", "1.0.0")
		c.Header("X-Knowledge-Answer-Contract-Version", entity.KnowledgeAnswerVersion)
		requestID := common.GenerateUUID()
		c.Set("knowledge_answer_request_id", requestID)
		c.Header("X-Request-ID", requestID)
		original := c.Writer
		buffer := &originalAuthWriter{ResponseWriter: original, status: http.StatusOK}
		c.Writer = buffer
		c.Set("knowledge_answer_writer", original)
		defer func() {
			if !c.GetBool("knowledge_answer_released") {
				c.Writer = original
				status, code := 401, "unauthenticated"
				if buffer.status == 503 {
					status, code = 503, "unavailable"
				}
				answerHTTPError(c, operation, requestID, status, code)
			}
		}()
		auth(c)
	}
}

func (h *DatasetsHandler) KnowledgeAnswer(operation string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if writer, ok := c.Get("knowledge_answer_writer"); ok {
			c.Writer = writer.(gin.ResponseWriter)
		}
		c.Set("knowledge_answer_released", true)
		requestID := c.GetString("knowledge_answer_request_id")
		defer func() {
			if recover() != nil && !c.Writer.Written() {
				answerHTTPError(c, operation, requestID, 503, "unavailable")
			}
		}()
		if c.GetBool("auth_via_api_token") {
			answerHTTPError(c, operation, requestID, 401, "unauthenticated")
			return
		}
		ctx, user, err := h.datasetsService.AuthenticateKnowledgeLocalSession(c.Request.Context(), c.GetHeader("Authorization"))
		if err != nil {
			answerHTTPError(c, operation, requestID, 401, "unauthenticated")
			return
		}
		actor, _ := c.Get("user")
		current, ok := actor.(*entity.User)
		if !ok || current == nil || current.ID != user {
			answerHTTPError(c, operation, requestID, 401, "unauthenticated")
			return
		}
		if c.GetHeader("X-Knowledge-Contract-Version") != "1.0.0" || c.GetHeader("X-Knowledge-Answer-Contract-Version") != entity.KnowledgeAnswerVersion {
			answerHTTPError(c, operation, requestID, 412, "contract_mismatch")
			return
		}
		media, _, issue := mime.ParseMediaType(c.GetHeader("Content-Type"))
		if issue != nil || media != "application/json" || c.Request.URL.RawQuery != "" {
			answerHTTPError(c, operation, requestID, 400, "invalid_request")
			return
		}
		raw, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 65536))
		if err != nil {
			answerHTTPError(c, operation, requestID, 400, "invalid_request")
			return
		}
		status, body := h.datasetsService.KnowledgeAnswer(ctx, user, c.Param("dataset_id"), operation, requestID, raw)
		c.Data(status, "application/json; charset=utf-8", body)
	}
}
