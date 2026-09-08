package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/asynchronomatic/speakeasy/pkg/proxy/modeldex"
)

// FIXME: alias until we restructure the code a bit
type OpenaiModel = modeldex.OpenaiModel
type OpenaiModelList = modeldex.OpenaiModelList

func (p *Proxy) openaiListModelsHandler(rpc *RPC) error {
	resp := &OpenaiModelList{
		Object: "list",
	}

	for _, model := range p.modelRouter.ListMeshModels() {
		resp.Data = append(resp.Data, OpenaiModel{
			ID:     model.Name,
			Object: "model",
			//Created: model.Properties.ModifiedAt.Unix(),
			OwnedBy: "ollama",
		})
	}
	return rpc.ReplyObject(resp)
}

// openaiAPIError is the nested object official OpenAI clients unmarshal
// from 4xx/5xx responses.
type openaiAPIError struct {
	Message string  `json:"message"`
	Type    string  `json:"type"`
	Param   *string `json:"param"`
	Code    string  `json:"code"`
}

type openaiAPIErrorBody struct {
	Error openaiAPIError `json:"error"`
}

func writeOpenAIError(w http.ResponseWriter, status int, errType, code, message string, param *string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(openaiAPIErrorBody{
		Error: openaiAPIError{
			Message: message,
			Type:    errType,
			Param:   param,
			Code:    code,
		},
	})
}

func modelNotFoundMessage(model string) string {
	if model == "" {
		return "The model does not exist or you do not have access to it."
	}
	return fmt.Sprintf("The model `%s` does not exist or you do not have access to it.", model)
}

func writeModelNotFound(w http.ResponseWriter, r *http.Request, model string) {
	if strings.HasPrefix(r.URL.Path, "/v1/") {
		writeOpenAIError(w, http.StatusNotFound, "invalid_request_error", "model_not_found", modelNotFoundMessage(model), nil)
		return
	}
	msg := "model not found"
	if model != "" {
		msg = fmt.Sprintf("model '%s' not found", model)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
