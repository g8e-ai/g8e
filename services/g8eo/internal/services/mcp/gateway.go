package mcp

import (
"net/http"

"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
)

type GatewayService struct {
}

func NewGatewayService() *GatewayService {
return &GatewayService{}
}

func (g *GatewayService) HandleToolsList(w http.ResponseWriter, r *http.Request) {
if r.Method != http.MethodPost && r.Method != http.MethodGet {
w.WriteHeader(http.StatusMethodNotAllowed)
return
}
w.Header().Set(constants.HeaderContentType, "application/json")
w.WriteHeader(http.StatusOK)
w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`))
}

func (g *GatewayService) HandleToolsCall(w http.ResponseWriter, r *http.Request) {
if r.Method != http.MethodPost {
w.WriteHeader(http.StatusMethodNotAllowed)
return
}
w.Header().Set(constants.HeaderContentType, "application/json")
w.WriteHeader(http.StatusOK)
w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"Execution paused. Please visit http://localhost:9000 to authorize via WebAuthn, then retry."}]}}`))
}
