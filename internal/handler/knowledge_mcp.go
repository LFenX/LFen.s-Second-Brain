package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"ragflow/internal/mcp"
	"ragflow/internal/service/dataset"
)

// KnowledgeMCPHandler adapts the native-session knowledge service to the API
// listener. Route registration and server lifetime remain with the API owner.
type KnowledgeMCPHandler struct {
	transport *mcp.Handler
}

func NewKnowledgeMCPHandler(service *dataset.DatasetService) (*KnowledgeMCPHandler, error) {
	if service == nil {
		return nil, errors.New("knowledge MCP service is required")
	}
	transport, err := mcp.NewKnowledgeHandler(mcp.KnowledgeOperations{
		Authenticate: service.AuthenticateKnowledgeLocalSession,
		Inspect:      service.InspectKnowledgeLocalIndex,
		Search:       service.SearchKnowledgeLocal,
		Read:         service.ReadKnowledgeLocal,
		Failure: func(err error) mcp.KnowledgeFailure {
			var public *dataset.KnowledgeLexicalError
			if errors.As(err, &public) {
				return mcp.KnowledgeFailure{Code: public.Code, OutcomeKnown: public.OutcomeKnown, RetrySameKey: public.RetrySameKey}
			}
			return mcp.KnowledgeFailure{Code: "unavailable", RetrySameKey: true}
		},
	})
	if err != nil {
		return nil, err
	}
	return &KnowledgeMCPHandler{transport: transport}, nil
}

func (h *KnowledgeMCPHandler) HandleMCP(c *gin.Context) {
	if c.Request.Method != http.MethodPost {
		c.Header("Allow", http.MethodPost)
		c.Status(http.StatusMethodNotAllowed)
		return
	}
	request := c.Request.Clone(c.Request.Context())
	request.URL.Path = "/mcp"
	request.URL.RawPath = ""
	h.transport.ServeHTTP(c.Writer, request)
}

func (h *KnowledgeMCPHandler) Close() error { return h.transport.Close() }

// KnowledgeMCP serves the static knowledge profile with this handler's dataset service.
func (h *DatasetsHandler) KnowledgeMCP(c *gin.Context) {
	profile, err := NewKnowledgeMCPHandler(h.datasetsService)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	defer profile.Close()
	profile.HandleMCP(c)
}
