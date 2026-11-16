package handler

import (
	"errors"

	"github.com/gin-gonic/gin"
	"ragflow/internal/common"
	"ragflow/internal/config"
	"ragflow/internal/entity"
	"ragflow/internal/mcp"
	"ragflow/internal/service/dataset"
)

var _ mcp.KnowledgeAccessOwner = (*dataset.DatasetService)(nil)

func knowledgeAccessMCPFailure(err error) mcp.KnowledgeAccessFailure {
	var failure *dataset.KnowledgeAccessError
	if errors.As(err, &failure) {
		return mcp.KnowledgeAccessFailure{Status: failure.Status, Code: failure.Code, OutcomeKnown: failure.OutcomeKnown, RetrySameKey: failure.RetrySameKey}
	}
	return mcp.KnowledgeAccessFailure{Status: 503, Code: "unavailable", RetrySameKey: true}
}

// KnowledgeAccessMCP binds only the new restricted bearer profile. Native
// session and owner MCP resolvers are not part of this dispatch path.
func (h *DatasetsHandler) KnowledgeAccessMCP(c *gin.Context) {
	audience, err := config.KnowledgeAccessMCPAudience()
	if err == nil && h != nil && h.datasetsService != nil {
		handler, createErr := mcp.NewKnowledgeAccessHandler(h.datasetsService, knowledgeAccessMCPFailure, audience)
		if createErr == nil {
			handler.ServeHTTP(c.Writer, c.Request)
			return
		}
	}
	c.Set("knowledge_access_request_id", common.GenerateUUID())
	c.Header("Cache-Control", "no-store")
	c.Header("X-Knowledge-Access-Version", entity.KnowledgeAccessVersion)
	c.Header("X-Request-ID", c.GetString("knowledge_access_request_id"))
	accessHandlerError(c, "mcpTransport", 503, "unavailable")
}
