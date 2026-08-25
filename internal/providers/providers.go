package providers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/SSNamahsos/Mihani-Code/internal/config"
)

// DiscoverModels supports OpenAI-compatible /models endpoints and returns IDs only.
func DiscoverModels(client *http.Client, baseURL, apiKey string) ([]string, error) {
	if client == nil {
		client = http.DefaultClient
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if !strings.HasSuffix(baseURL, "/v1") && !strings.HasSuffix(baseURL, "/api/v1") {
		baseURL += "/v1"
	}
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
	return config.Provider{Label: name, Type: "openai", BaseURL: strings.TrimRight(baseURL, "/"), APIKey: apiKey, Models: models}
}
