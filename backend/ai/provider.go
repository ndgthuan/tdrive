package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

// Image is one picture attached to a turn. Data holds raw JPEG (or other
// image) bytes; providers base64-encode it.
type Image struct {
	MIMEType string
	Data     []byte
}

// Turn is one message in the conversation sent to a provider. Role is "user"
// or "assistant".
type Turn struct {
	Role   string
	Text   string
	Images []Image
}

// Request is the provider-neutral completion request.
type Request struct {
	// System is the system prompt. Empty means none.
	System string
	Turns  []Turn
	// JSONOutput asks the provider for a JSON object reply where supported.
	JSONOutput bool
	// MaxTokens caps the reply length. Zero picks a provider default.
	MaxTokens int
}

// Reply is the assistant text returned by a provider.
type Reply struct {
	Text string
}

// Completer is one configured provider endpoint.
type Completer interface {
	Complete(ctx context.Context, req Request) (Reply, error)
}

// ErrInvalidAPIKey reports a 401 or 403 from the provider.
var ErrInvalidAPIKey = errors.New("invalid API key")

// ErrRateLimited reports a 429 from the provider.
var ErrRateLimited = errors.New("rate limited by provider; try again shortly")

// ErrProviderBlocked reports a reply the provider withheld for safety reasons.
var ErrProviderBlocked = errors.New("reply blocked by provider safety filter")

// APIError is a non-success response from a provider. It never includes the
// API key; the provider's own message is trimmed and kept for context.
type APIError struct {
	Provider string
	Status   int
	Message  string
	cause    error
}

func (e *APIError) Error() string {
	text := fmt.Sprintf("%s: %s (HTTP %d)", e.Provider, e.cause.Error(), e.Status)
	if e.Message != "" {
		text += ": " + e.Message
	}
	return text
}

func (e *APIError) Unwrap() error { return e.cause }

const (
	default_chat_timeout   = 120 * time.Second
	max_response_bytes     = 16 << 20
	max_error_message_rune = 300
)

// NewCompleter builds the provider client for a resolved profile. http_client
// may be nil; a client with a sane timeout is used instead.
func NewCompleter(runtime ModelRuntime, http_client *http.Client) (Completer, error) {
	if http_client == nil {
		http_client = &http.Client{Timeout: default_chat_timeout}
	}
	profile := runtime.Profile
	base := profile.BaseURL
	if base == "" {
		base = default_base_url(profile.Provider)
	}
	if base == "" {
		return nil, errors.New("ai: base URL is required for a custom provider")
	}
	if profile.Model == "" {
		return nil, errors.New("ai: model name is required")
	}
	switch profile.Provider {
	case ProviderOpenAI, ProviderOpenRouter, ProviderCustom:
		return &openai_completer{
			provider: string(profile.Provider),
			base:     base,
			model:    profile.Model,
			api_key:  runtime.APIKey,
			client:   http_client,
		}, nil
	case ProviderAnthropic:
		return &anthropic_completer{base: base, model: profile.Model, api_key: runtime.APIKey, client: http_client}, nil
	case ProviderGemini:
		return &gemini_completer{base: base, model: profile.Model, api_key: runtime.APIKey, client: http_client}, nil
	default:
		return nil, fmt.Errorf("ai: unknown provider %q", profile.Provider)
	}
}

func default_base_url(provider Provider) string {
	switch provider {
	case ProviderOpenAI:
		return "https://api.openai.com/v1"
	case ProviderOpenRouter:
		return "https://openrouter.ai/api/v1"
	case ProviderAnthropic:
		return "https://api.anthropic.com"
	case ProviderGemini:
		return "https://generativelanguage.googleapis.com/v1beta"
	default:
		return ""
	}
}

// post_json sends body as JSON and returns the response body. Non-2xx responses
// are turned into APIError values with a clear cause.
func post_json(ctx context.Context, client *http.Client, provider, url string, headers map[string]string, body any) ([]byte, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("%s: encode request: %w", provider, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("%s: build request: %w", provider, err)
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%s: request failed: %w", provider, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, max_response_bytes))
	if err != nil {
		return nil, fmt.Errorf("%s: read response: %w", provider, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, api_error(provider, resp.StatusCode, data)
	}
	return data, nil
}

// api_error maps an HTTP status to a stable, human-readable error.
func api_error(provider string, status int, body []byte) error {
	var cause error
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		cause = ErrInvalidAPIKey
	case status == http.StatusTooManyRequests:
		cause = ErrRateLimited
	case status == http.StatusNotFound:
		cause = errors.New("model or endpoint not found; check the model name and base URL")
	case status == http.StatusBadRequest:
		cause = errors.New("request rejected")
	case status >= 500:
		cause = errors.New("provider server error; try again later")
	default:
		cause = errors.New("unexpected response")
	}
	return &APIError{Provider: provider, Status: status, Message: provider_message(body), cause: cause}
}

// provider_message pulls error.message out of the common {"error":{...}} shape
// used by OpenAI-compatible, Anthropic and Gemini endpoints.
func provider_message(body []byte) string {
	var envelope struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.Error.Message == "" {
		return ""
	}
	msg := strings.TrimSpace(envelope.Error.Message)
	if utf8.RuneCountInString(msg) > max_error_message_rune {
		runes := []rune(msg)
		msg = string(runes[:max_error_message_rune]) + "..."
	}
	return msg
}

// image_data_url renders an image as a data: URL for OpenAI-style APIs.
func image_data_url(img Image) string {
	return "data:" + image_mime(img) + ";base64," + base64.StdEncoding.EncodeToString(img.Data)
}

func image_mime(img Image) string {
	if img.MIMEType == "" {
		return "image/jpeg"
	}
	return img.MIMEType
}

// max_tokens_or returns the requested cap or the fallback when unset.
func max_tokens_or(requested, fallback int) int {
	if requested > 0 {
		return requested
	}
	return fallback
}

// openai_completer speaks POST {base}/chat/completions. It serves OpenAI,
// OpenRouter and any OpenAI-compatible custom endpoint.
type openai_completer struct {
	provider string
	base     string
	model    string
	api_key  string
	client   *http.Client
}

type openai_message struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type openai_response struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (c *openai_completer) Complete(ctx context.Context, req Request) (Reply, error) {
	messages := make([]openai_message, 0, len(req.Turns)+1)
	if req.System != "" {
		messages = append(messages, openai_message{Role: "system", Content: req.System})
	}
	for _, turn := range req.Turns {
		if len(turn.Images) == 0 {
			messages = append(messages, openai_message{Role: turn.Role, Content: turn.Text})
			continue
		}
		parts := make([]map[string]any, 0, len(turn.Images)+1)
		if turn.Text != "" {
			parts = append(parts, map[string]any{"type": "text", "text": turn.Text})
		}
		for _, img := range turn.Images {
			parts = append(parts, map[string]any{
				"type":      "image_url",
				"image_url": map[string]string{"url": image_data_url(img)},
			})
		}
		messages = append(messages, openai_message{Role: turn.Role, Content: parts})
	}
	body := map[string]any{
		"model":      c.model,
		"messages":   messages,
		"max_tokens": max_tokens_or(req.MaxTokens, 2048),
	}
	if req.JSONOutput {
		body["response_format"] = map[string]string{"type": "json_object"}
	}
	headers := map[string]string{}
	if c.api_key != "" {
		headers["Authorization"] = "Bearer " + c.api_key
	}
	data, err := post_json(ctx, c.client, c.provider, c.base+"/chat/completions", headers, body)
	if err != nil {
		return Reply{}, err
	}
	var out openai_response
	if err := json.Unmarshal(data, &out); err != nil {
		return Reply{}, fmt.Errorf("%s: decode reply: %w", c.provider, err)
	}
	if len(out.Choices) == 0 || strings.TrimSpace(out.Choices[0].Message.Content) == "" {
		return Reply{}, fmt.Errorf("%s: empty reply", c.provider)
	}
	return Reply{Text: out.Choices[0].Message.Content}, nil
}

// anthropic_completer speaks POST {base}/v1/messages with the 2023-06-01 API
// version header.
type anthropic_completer struct {
	base    string
	model   string
	api_key string
	client  *http.Client
}

type anthropic_response struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

func (c *anthropic_completer) Complete(ctx context.Context, req Request) (Reply, error) {
	messages := make([]map[string]any, 0, len(req.Turns))
	for _, turn := range req.Turns {
		blocks := make([]map[string]any, 0, len(turn.Images)+1)
		for _, img := range turn.Images {
			blocks = append(blocks, map[string]any{
				"type": "image",
				"source": map[string]string{
					"type":       "base64",
					"media_type": image_mime(img),
					"data":       base64.StdEncoding.EncodeToString(img.Data),
				},
			})
		}
		if turn.Text != "" || len(blocks) == 0 {
			blocks = append(blocks, map[string]any{"type": "text", "text": turn.Text})
		}
		messages = append(messages, map[string]any{"role": turn.Role, "content": blocks})
	}
	body := map[string]any{
		"model":      c.model,
		"max_tokens": max_tokens_or(req.MaxTokens, 2048),
		"messages":   messages,
	}
	if req.System != "" {
		body["system"] = req.System
	}
	headers := map[string]string{
		"x-api-key":         c.api_key,
		"anthropic-version": "2023-06-01",
	}
	data, err := post_json(ctx, c.client, "anthropic", c.base+"/v1/messages", headers, body)
	if err != nil {
		return Reply{}, err
	}
	var out anthropic_response
	if err := json.Unmarshal(data, &out); err != nil {
		return Reply{}, fmt.Errorf("anthropic: decode reply: %w", err)
	}
	var text strings.Builder
	for _, block := range out.Content {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
	}
	if strings.TrimSpace(text.String()) == "" {
		return Reply{}, errors.New("anthropic: empty reply")
	}
	return Reply{Text: text.String()}, nil
}

// gemini_completer speaks POST {base}/models/{model}:generateContent.
type gemini_completer struct {
	base    string
	model   string
	api_key string
	client  *http.Client
}

type gemini_response struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	PromptFeedback struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
}

func (c *gemini_completer) Complete(ctx context.Context, req Request) (Reply, error) {
	contents := make([]map[string]any, 0, len(req.Turns))
	for _, turn := range req.Turns {
		role := "user"
		if turn.Role == "assistant" {
			role = "model"
		}
		parts := make([]map[string]any, 0, len(turn.Images)+1)
		for _, img := range turn.Images {
			parts = append(parts, map[string]any{
				"inlineData": map[string]string{
					"mimeType": image_mime(img),
					"data":     base64.StdEncoding.EncodeToString(img.Data),
				},
			})
		}
		if turn.Text != "" || len(parts) == 0 {
			parts = append(parts, map[string]any{"text": turn.Text})
		}
		contents = append(contents, map[string]any{"role": role, "parts": parts})
	}
	generation := map[string]any{"maxOutputTokens": max_tokens_or(req.MaxTokens, 2048)}
	if req.JSONOutput {
		generation["responseMimeType"] = "application/json"
	}
	body := map[string]any{
		"contents":         contents,
		"generationConfig": generation,
	}
	if req.System != "" {
		body["systemInstruction"] = map[string]any{"parts": []map[string]string{{"text": req.System}}}
	}
	model := strings.TrimPrefix(c.model, "models/")
	headers := map[string]string{"x-goog-api-key": c.api_key}
	url := c.base + "/models/" + model + ":generateContent"
	data, err := post_json(ctx, c.client, "gemini", url, headers, body)
	if err != nil {
		return Reply{}, err
	}
	var out gemini_response
	if err := json.Unmarshal(data, &out); err != nil {
		return Reply{}, fmt.Errorf("gemini: decode reply: %w", err)
	}
	if out.PromptFeedback.BlockReason != "" {
		return Reply{}, ErrProviderBlocked
	}
	if len(out.Candidates) == 0 {
		return Reply{}, errors.New("gemini: empty reply")
	}
	var text strings.Builder
	for _, part := range out.Candidates[0].Content.Parts {
		text.WriteString(part.Text)
	}
	if strings.TrimSpace(text.String()) == "" {
		return Reply{}, errors.New("gemini: empty reply")
	}
	return Reply{Text: text.String()}, nil
}
