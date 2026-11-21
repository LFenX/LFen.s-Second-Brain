package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/server/local"
)

func accessHandlerError(c *gin.Context, operation string, status int, code string) {
	body := entity.KnowledgeAccessError{Operation: operation, ContractVersion: entity.KnowledgeAccessVersion, RequestID: c.GetString("knowledge_access_request_id"), Code: code, Message: "Knowledge access operation could not be completed", OutcomeKnown: status != 503, RetrySameKey: status == 503}
	raw, _ := json.Marshal(body)
	c.Data(status, "application/json; charset=utf-8", raw)
	c.Abort()
}

// KnowledgeAccessAuth authenticates only a native local session. API and beta
// token resolvers are never reached from this private entry.
func (h *AuthHandler) KnowledgeAccessAuth(operation string) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(dao.KnowledgeAccessPrivateReadContext(c.Request.Context()), 15*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Header("Cache-Control", "no-store")
		c.Header("X-Knowledge-Access-Version", entity.KnowledgeAccessVersion)
		requestID := common.GenerateUUID()
		c.Set("knowledge_access_request_id", requestID)
		c.Header("X-Request-ID", requestID)
		defer func() {
			if recover() != nil && !c.Writer.Written() {
				accessHandlerError(c, operation, 503, "unavailable")
			}
		}()
		if c.Request.Method != http.MethodPost {
			c.Header("Allow", http.MethodPost)
			accessHandlerError(c, operation, 405, "invalid_request")
			return
		}
		authorizations := c.Request.Header.Values("Authorization")
		if len(authorizations) != 1 || strings.TrimSpace(authorizations[0]) == "" {
			accessHandlerError(c, operation, 401, "unauthenticated")
			return
		}
		authorization := authorizations[0]
		identity, code, err := h.userService.ResolveAccessToken(ctx, authorization)
		if err != nil || code != common.CodeSuccess || identity == nil || identity.User == nil || identity.User.IsSuperuser != nil && *identity.User.IsSuperuser {
			accessHandlerError(c, operation, 401, "unauthenticated")
			return
		}
		if !local.IsAdminAvailable() {
			accessHandlerError(c, operation, 503, "unavailable")
			return
		}
		setAuthIdentity(c, identity)
		c.Set("email", identity.User.Email)
		c.Set("auth_via_api_token", false)
		c.Next()
	}
}

func (h *DatasetsHandler) KnowledgeAccess(operation string) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recover() != nil && !c.Writer.Written() {
				accessHandlerError(c, operation, 503, "unavailable")
			}
		}()
		if c.GetBool("auth_via_api_token") {
			accessHandlerError(c, operation, 401, "unauthenticated")
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()
		ctx, user, err := h.datasetsService.AuthenticateKnowledgeLocalSession(ctx, c.GetHeader("Authorization"))
		if err != nil {
			accessHandlerError(c, operation, 401, "unauthenticated")
			return
		}
		actor, _ := c.Get("user")
		current, ok := actor.(*entity.User)
		if !ok || current == nil || current.ID != user {
			accessHandlerError(c, operation, 401, "unauthenticated")
			return
		}
		if c.Request.URL.RawQuery != "" {
			accessHandlerError(c, operation, 400, "invalid_request")
			return
		}
		media, params, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
		if err != nil || media != "application/json" || params["charset"] != "" && !strings.EqualFold(params["charset"], "utf-8") {
			accessHandlerError(c, operation, 400, "invalid_request")
			return
		}
		raw, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 32768))
		if err != nil {
			var limit *http.MaxBytesError
			if errors.As(err, &limit) {
				accessHandlerError(c, operation, 413, "capacity_exceeded")
			} else {
				accessHandlerError(c, operation, 400, "invalid_request")
			}
			return
		}
		status, response := h.datasetsService.KnowledgeAccess(ctx, user, operation, c.GetString("knowledge_access_request_id"), raw)
		c.Data(status, "application/json; charset=utf-8", response)
	}
}
