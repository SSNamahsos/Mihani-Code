package providers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/SSNamahsos/Mihani-Code/internal/config"
)

// NormalizeBaseURL canonicalizes an OpenAI-compatible base URL so the same
// value works for both model discovery and chat requests. Gateways expose
// their API under /v1 (or /api/v1); users commonly paste the bare domain.
func NormalizeBaseURL(base string) string {
	base = strings.TrimRight(base, "/")
	if base != "" && !strings.HasSuffix(base, "/v1") && !strings.HasSuffix(base, "/api/v1") {
		base += "/v1"
	}
	return base
}

// DiscoverModels supports OpenAI-compatible /models endpoints and returns IDs only.
func DiscoverModels(client *http.Client, baseURL, apiKey string) ([]string, error) {
	if client == nil {
		client = http.DefaultClient
	}
	baseURL = NormalizeBaseURL(baseURL)
	req, err := http.NewRequest(http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("models endpoint returned %s", resp.Status)
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	models := make([]string, 0, len(payload.Data))
	for _, model := range payload.Data {
		if model.ID != "" {
			models = append(models, model.ID)
		}
	}
	return models, nil
}

func NormalizeProvider(name, baseURL, apiKey string, models []string) config.Provider {
	return config.Provider{
		Label:       name,
		Type:        "openai",
		BaseURL:     NormalizeBaseURL(baseURL),
		APIKey:      apiKey,
		Models:      models,
		NativeTools: config.NativeToolsDefault(baseURL),
	}
}
