package upstream

import (
	"net/http"

	"kiro2api/internal/adapter/upstream/anthropic"
	"kiro2api/internal/adapter/upstream/openai"
	"kiro2api/internal/adapter/upstream/shared"
	"kiro2api/types"

	"github.com/gin-gonic/gin"
)

type Gateway struct {
	reverseProxy *shared.ReverseProxy
	anthropic    *anthropic.Proxy
	openai       *openai.Proxy
}

func NewGateway() *Gateway {
	reverseProxy := shared.NewReverseProxy(nil)
	return &Gateway{
		reverseProxy: reverseProxy,
		anthropic:    anthropic.NewProxy(reverseProxy),
		openai:       openai.NewProxy(reverseProxy),
	}
}

func (g *Gateway) HandleAnthropicStream(c *gin.Context, req types.AnthropicRequest, token *types.TokenWithUsage) {
	g.anthropic.HandleStream(c, req, token)
}

func (g *Gateway) HandleAnthropicNonStream(c *gin.Context, req types.AnthropicRequest, token types.TokenInfo) {
	g.anthropic.HandleNonStream(c, req, token)
}

func (g *Gateway) HandleOpenAINonStream(c *gin.Context, req types.AnthropicRequest, token types.TokenInfo) {
	g.openai.HandleNonStream(c, req, token)
}

func (g *Gateway) HandleOpenAIStream(c *gin.Context, req types.AnthropicRequest, token types.TokenInfo) {
	g.openai.HandleStream(c, req, token)
}

func (g *Gateway) ExecuteCodeWhispererRequest(c *gin.Context, req types.AnthropicRequest, token types.TokenInfo, isStream bool) (*http.Response, error) {
	return g.reverseProxy.Execute(c, req, token, isStream)
}
