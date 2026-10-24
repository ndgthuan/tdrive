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

// ToolSpec declares one tool the model may call. Parameters is a JSON Schema
// object. Each provider adapter converts it to its own wire format.
type ToolSpec struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// ToolCall is one tool invocation requested by the model. Arguments is the
// JSON object text the model produced. ID echoes the provider's call id;
// Gemini has none, so ID is the function name there. Signature is Gemini's
// thought signature, which must be sent back with the call.
type ToolCall struct {
	ID        string
	Name      string
	Arguments string
	Signature string
}

// ToolResult answers one ToolCall. Images carry media the model should look
// at; adapters place them where their wire format allows.
type ToolResult struct {
	CallID  string
	Name    string
	Content string
	IsError bool
	Images  []Image
}

// Turn is one message in the conversation sent to a provider. Role is "user"
// or "assistant". An assistant turn may carry ToolCalls; a user turn may carry
// ToolResults answering the previous assistant turn.
type Turn struct {
	Role        string
	Text        string
	Images      []Image
	ToolCalls   []ToolCall
	ToolResults []ToolResult
}

// Request is the provider-neutral completion request.
type Request struct {
	// System is the system prompt. Empty means none.
	System string
	Turns  []Turn
	// Tools lists the tools the model may call. Empty sends no tools.
	Tools []ToolSpec
	// JSONOutput asks the provider for a JSON object reply where supported.
	JSONOutput bool
	// MaxTokens caps the reply length. Zero picks a provider default.
	MaxTokens int
}

// Reply is what a provider returned: assistant text and/or tool calls.
type Reply struct {
	Text      string
	ToolCalls []ToolCall
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

// arguments_object returns tool arguments as a JSON object, defaulting to {}.
func arguments_object(text string) json.RawMessage {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || !json.Valid([]byte(trimmed)) {
		return json.RawMessage("{}")
	}
	return json.RawMessage(trimmed)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
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
	Role       string            `json:"role"`
	Content    any               `json:"content"`
	ToolCalls  []openai_tool_out `json:"tool_calls,omitempty"`
	ToolCallID string            `json:"tool_call_id,omitempty"`
}

type openai_tool_out struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openai_response struct {
	Choices []struct {
		Message struct {
			Content   *string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
}

func (c *openai_completer) Complete(ctx context.Context, req Request) (Reply, error) {
	messages := make([]openai_message, 0, len(req.Turns)+1)
	if req.System != "" {
		messages = append(messages, openai_message{Role: "system", Content: req.System})
	}
	for _, turn := range req.Turns {
		switch {
		case len(turn.ToolCalls) > 0:
			msg := openai_message{Role: "assistant"}
			if turn.Text != "" {
				msg.Content = turn.Text
			}
			for _, call := range turn.ToolCalls {
				var out openai_tool_out
				out.ID = call.ID
				out.Type = "function"
				out.Function.Name = call.Name
				out.Function.Arguments = string(arguments_object(call.Arguments))
				msg.ToolCalls = append(msg.ToolCalls, out)
			}
			messages = append(messages, msg)
		case len(turn.ToolResults) > 0:
			// Tool results are text-only in this format, so any media goes into
			// one user message that follows them.
			var media []any
			for _, result := range turn.ToolResults {
				content := result.Content
				if result.IsError {
					content = "Error: " + content
				}
				messages = append(messages, openai_message{Role: "tool", Content: content, ToolCallID: result.CallID})
				for _, img := range result.Images {
					media = append(media, map[string]any{"type": "image_url", "image_url": map[string]string{"url": image_data_url(img)}})
				}
			}
			if len(media) > 0 || turn.Text != "" {
				parts := []any{map[string]any{"type": "text", "text": firstNonEmpty(turn.Text, "Media returned by the tool call above:")}}
				parts = append(parts, media...)
				messages = append(messages, openai_message{Role: "user", Content: parts})
			}
		case len(turn.Images) == 0:
			messages = append(messages, openai_message{Role: turn.Role, Content: turn.Text})
		default:
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
	}
	body := map[string]any{
		"model":      c.model,
		"messages":   messages,
		"max_tokens": max_tokens_or(req.MaxTokens, 2048),
	}
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, 0, len(req.Tools))
		for _, tool := range req.Tools {
			tools = append(tools, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        tool.Name,
					"description": tool.Description,
					"parameters":  tool.Parameters,
				},
			})
		}
		body["tools"] = tools
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
	if len(out.Choices) == 0 {
		return Reply{}, fmt.Errorf("%s: empty reply", c.provider)
	}
	message := out.Choices[0].Message
	reply := Reply{}
	if message.Content != nil {
		reply.Text = *message.Content
	}
	for i, call := range message.ToolCalls {
		id := call.ID
		if id == "" {
			id = fmt.Sprintf("call_%d", i)
		}
		reply.ToolCalls = append(reply.ToolCalls, ToolCall{ID: id, Name: call.Function.Name, Arguments: call.Function.Arguments})
	}
	if strings.TrimSpace(reply.Text) == "" && len(reply.ToolCalls) == 0 {
		return Reply{}, fmt.Errorf("%s: empty reply", c.provider)
	}
	return reply, nil
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
		Type  string          `json:"type"`
		Text  string          `json:"text"`
		ID    string          `json:"id"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	} `json:"content"`
}

func anthropic_image_block(img Image) map[string]any {
	return map[string]any{
		"type": "image",
		"source": map[string]string{
			"type":       "base64",
			"media_type": image_mime(img),
			"data":       base64.StdEncoding.EncodeToString(img.Data),
		},
	}
}

func (c *anthropic_completer) Complete(ctx context.Context, req Request) (Reply, error) {
	messages := make([]map[string]any, 0, len(req.Turns))
	for _, turn := range req.Turns {
		var blocks []map[string]any
		role := turn.Role
		switch {
		case len(turn.ToolCalls) > 0:
			if turn.Text != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": turn.Text})
			}
			for _, call := range turn.ToolCalls {
				blocks = append(blocks, map[string]any{
					"type":  "tool_use",
					"id":    call.ID,
					"name":  call.Name,
					"input": arguments_object(call.Arguments),
				})
			}
		case len(turn.ToolResults) > 0:
			role = "user"
			for _, result := range turn.ToolResults {
				content := []map[string]any{{"type": "text", "text": result.Content}}
				for _, img := range result.Images {
					content = append(content, anthropic_image_block(img))
				}
				block := map[string]any{"type": "tool_result", "tool_use_id": result.CallID, "content": content}
				if result.IsError {
					block["is_error"] = true
				}
				blocks = append(blocks, block)
			}
			if turn.Text != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": turn.Text})
			}
		default:
			for _, img := range turn.Images {
				blocks = append(blocks, anthropic_image_block(img))
			}
			if turn.Text != "" || len(blocks) == 0 {
				blocks = append(blocks, map[string]any{"type": "text", "text": turn.Text})
			}
		}
		messages = append(messages, map[string]any{"role": role, "content": blocks})
	}
	body := map[string]any{
		"model":      c.model,
		"max_tokens": max_tokens_or(req.MaxTokens, 2048),
		"messages":   messages,
	}
	if req.System != "" {
		body["system"] = req.System
	}
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, 0, len(req.Tools))
		for _, tool := range req.Tools {
			tools = append(tools, map[string]any{
				"name":         tool.Name,
				"description":  tool.Description,
				"input_schema": tool.Parameters,
			})
		}
		body["tools"] = tools
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
	var reply Reply
	var text strings.Builder
	for _, block := range out.Content {
		switch block.Type {
		case "text":
			text.WriteString(block.Text)
		case "tool_use":
			reply.ToolCalls = append(reply.ToolCalls, ToolCall{ID: block.ID, Name: block.Name, Arguments: string(arguments_object(string(block.Input)))})
		}
	}
	reply.Text = text.String()
	if strings.TrimSpace(reply.Text) == "" && len(reply.ToolCalls) == 0 {
		return Reply{}, errors.New("anthropic: empty reply")
	}
	return reply, nil
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
				Text             string `json:"text"`
				ThoughtSignature string `json:"thoughtSignature"`
				FunctionCall     *struct {
					Name string          `json:"name"`
					Args json.RawMessage `json:"args"`
				} `json:"functionCall"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	PromptFeedback struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
}

// gemini_schema converts a JSON Schema to the OpenAPI subset Gemini accepts:
// type names are upper-cased and additionalProperties and $schema are dropped.
func gemini_schema(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			if key == "additionalProperties" || key == "$schema" {
				continue
			}
			if s, ok := item.(string); ok && key == "type" {
				out[key] = strings.ToUpper(s)
				continue
			}
			out[key] = gemini_schema(item)
		}
		return out
	case []any:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, gemini_schema(item))
		}
		return out
	default:
		return value
	}
}

func (c *gemini_completer) Complete(ctx context.Context, req Request) (Reply, error) {
	contents := make([]map[string]any, 0, len(req.Turns))
	for _, turn := range req.Turns {
		role := "user"
		if turn.Role == "assistant" {
			role = "model"
		}
		var parts []map[string]any
		switch {
		case len(turn.ToolCalls) > 0:
			role = "model"
			if turn.Text != "" {
				parts = append(parts, map[string]any{"text": turn.Text})
			}
			for _, call := range turn.ToolCalls {
				part := map[string]any{"functionCall": map[string]any{"name": call.Name, "args": arguments_object(call.Arguments)}}
				if call.Signature != "" {
					part["thoughtSignature"] = call.Signature
				}
				parts = append(parts, part)
			}
		case len(turn.ToolResults) > 0:
			var media []map[string]any
			for _, result := range turn.ToolResults {
				key := "result"
				if result.IsError {
					key = "error"
				}
				parts = append(parts, map[string]any{"functionResponse": map[string]any{
					"name":     result.Name,
					"response": map[string]any{key: result.Content},
				}})
				for _, img := range result.Images {
					media = append(media, map[string]any{"inlineData": map[string]string{
						"mimeType": image_mime(img),
						"data":     base64.StdEncoding.EncodeToString(img.Data),
					}})
				}
			}
			parts = append(parts, media...)
			if turn.Text != "" {
				parts = append(parts, map[string]any{"text": turn.Text})
			}
		default:
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
	if len(req.Tools) > 0 {
		declarations := make([]map[string]any, 0, len(req.Tools))
		for _, tool := range req.Tools {
			declarations = append(declarations, map[string]any{
				"name":        tool.Name,
				"description": tool.Description,
				"parameters":  gemini_schema(tool.Parameters),
			})
		}
		body["tools"] = []map[string]any{{"functionDeclarations": declarations}}
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
	var reply Reply
	var text strings.Builder
	for _, part := range out.Candidates[0].Content.Parts {
		text.WriteString(part.Text)
		if part.FunctionCall != nil {
			reply.ToolCalls = append(reply.ToolCalls, ToolCall{
				ID:        part.FunctionCall.Name,
				Name:      part.FunctionCall.Name,
				Arguments: string(arguments_object(string(part.FunctionCall.Args))),
				Signature: part.ThoughtSignature,
			})
		}
	}
	reply.Text = text.String()
	if strings.TrimSpace(reply.Text) == "" && len(reply.ToolCalls) == 0 {
		return Reply{}, errors.New("gemini: empty reply")
	}
	return reply, nil
}
