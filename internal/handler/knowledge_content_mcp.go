package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"ragflow/internal/mcp"
	"ragflow/internal/service/dataset"
)

type KnowledgeContentMCPHandler struct{ transport *mcp.Handler }

// NewKnowledgeContentMCPHandler binds the profile to the owning DatasetService.
func NewKnowledgeContentMCPHandler(owner *dataset.DatasetService) (*KnowledgeContentMCPHandler, error) {
	if owner == nil {
		return nil, errors.New("owning DatasetService is required")
	}
	transport, err := mcp.NewContentKnowledgeHandler(owner, contentKnowledgeMCPFailure)
	if err != nil {
		return nil, err
	}
	return &KnowledgeContentMCPHandler{transport: transport}, nil
}

func (h *KnowledgeContentMCPHandler) HandleMCP(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if c.Request.Method != http.MethodPost {
		c.Header("Allow", http.MethodPost)
		c.Status(http.StatusMethodNotAllowed)
		return
	}
	request := c.Request.Clone(c.Request.Context())
	request.URL.Path, request.URL.RawPath = "/mcp", ""
	h.transport.ServeHTTP(c.Writer, request)
}

func (h *KnowledgeContentMCPHandler) Close() error { return h.transport.Close() }

func contentKnowledgeMCPFailure(err error) mcp.KnowledgeFailure {
	var public *dataset.KnowledgeLexicalError
	if errors.As(err, &public) {
		return mcp.KnowledgeFailure{Code: public.Code, OutcomeKnown: public.OutcomeKnown, RetrySameKey: public.RetrySameKey}
	}
	return mcp.KnowledgeFailure{Code: "unavailable", RetrySameKey: true}
}

// KnowledgeContentMCP uses the same DatasetService instance as dataset routes.
// The stateless JSON transport lives for one request and is always closed.
func (h *DatasetsHandler) KnowledgeContentMCP(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	profile, err := NewKnowledgeContentMCPHandler(h.datasetsService)
	if err != nil {
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return
	}
	defer profile.Close()
	profile.HandleMCP(c)
}
