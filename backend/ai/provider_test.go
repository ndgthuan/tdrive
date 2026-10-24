package ai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

var test_jpeg = []byte{0xff, 0xd8, 0xff, 0xe0, 1, 2, 3}

// capture_server records the last request and replies with body and status.
func capture_server(t *testing.T, status int, body string) (*httptest.Server, *captured_request) {
	t.Helper()
	captured := &captured_request{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		captured.method = r.Method
		captured.path = r.URL.Path
		captured.query = r.URL.RawQuery
		captured.headers = r.Header.Clone()
		captured.body = data
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return server, captured
}

type captured_request struct {
	method  string
	path    string
	query   string
	headers http.Header
	body    []byte
}

func (c *captured_request) json_body(t *testing.T) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(c.body, &out); err != nil {
		t.Fatalf("request body is not JSON: %v (%s)", err, c.body)
	}
	return out
}

func test_request() Request {
	return Request{
		System: "be brief",
		Turns: []Turn{
			{Role: "user", Text: "describe", Images: []Image{{MIMEType: "image/jpeg", Data: test_jpeg}}},
			{Role: "assistant", Text: "previous answer"},
			{Role: "user", Text: "again"},
		},
		JSONOutput: true,
		MaxTokens:  100,
	}
}

func TestOpenAICompatibleRequestFormat(t *testing.T) {
	server, captured := capture_server(t, http.StatusOK, `{"choices":[{"message":{"content":"a cat"}}]}`)
	completer, err := NewCompleter(ModelRuntime{
		Profile: ModelProfile{Provider: ProviderCustom, BaseURL: server.URL + "/v1", Model: "local-vision"},
		APIKey:  "sk-test",
	}, nil)
	if err != nil {
		t.Fatalf("new completer: %v", err)
	}
	reply, err := completer.Complete(context.Background(), test_request())
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if reply.Text != "a cat" {
		t.Fatalf("reply = %q", reply.Text)
	}
	if captured.path != "/v1/chat/completions" {
		t.Fatalf("path = %s", captured.path)
	}
	if got := captured.headers.Get("Authorization"); got != "Bearer sk-test" {
		t.Fatalf("authorization = %q", got)
	}
	body := captured.json_body(t)
	if body["model"] != "local-vision" || body["max_tokens"].(float64) != 100 {
		t.Fatalf("model/max_tokens wrong: %v", body)
	}
	if rf := body["response_format"].(map[string]any); rf["type"] != "json_object" {
		t.Fatalf("response_format = %v", rf)
	}
	messages := body["messages"].([]any)
	if len(messages) != 4 {
		t.Fatalf("want system + 3 turns, got %d", len(messages))
	}
	if messages[0].(map[string]any)["role"] != "system" {
		t.Fatalf("first message must be system")
	}
	first := messages[1].(map[string]any)["content"].([]any)
	image := first[1].(map[string]any)
	if image["type"] != "image_url" {
		t.Fatalf("second part should be image_url: %v", image)
	}
	url := image["image_url"].(map[string]any)["url"].(string)
	want := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(test_jpeg)
	if url != want {
		t.Fatalf("image data URL = %q", url)
	}
}

func TestOpenAIOmitsAuthorizationWithoutKey(t *testing.T) {
	server, captured := capture_server(t, http.StatusOK, `{"choices":[{"message":{"content":"ok"}}]}`)
	completer, err := NewCompleter(ModelRuntime{Profile: ModelProfile{Provider: ProviderCustom, BaseURL: server.URL, Model: "m"}}, nil)
	if err != nil {
		t.Fatalf("new completer: %v", err)
	}
	if _, err := completer.Complete(context.Background(), Request{Turns: []Turn{{Role: "user", Text: "hi"}}}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if captured.headers.Get("Authorization") != "" {
		t.Fatalf("no key means no Authorization header")
	}
}

func TestAnthropicRequestFormat(t *testing.T) {
	server, captured := capture_server(t, http.StatusOK, `{"content":[{"type":"text","text":"a "},{"type":"text","text":"dog"}]}`)
	completer, err := NewCompleter(ModelRuntime{
		Profile: ModelProfile{Provider: ProviderAnthropic, BaseURL: server.URL, Model: "claude-test"},
		APIKey:  "ant-key",
	}, nil)
	if err != nil {
		t.Fatalf("new completer: %v", err)
	}
	reply, err := completer.Complete(context.Background(), test_request())
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if reply.Text != "a dog" {
		t.Fatalf("reply = %q", reply.Text)
	}
	if captured.path != "/v1/messages" {
		t.Fatalf("path = %s", captured.path)
	}
	if captured.headers.Get("x-api-key") != "ant-key" {
		t.Fatalf("x-api-key header missing")
	}
	if captured.headers.Get("anthropic-version") != "2023-06-01" {
		t.Fatalf("anthropic-version = %q", captured.headers.Get("anthropic-version"))
	}
	body := captured.json_body(t)
	if body["system"] != "be brief" || body["max_tokens"].(float64) != 100 {
		t.Fatalf("system/max_tokens wrong: %v", body)
	}
	messages := body["messages"].([]any)
	if len(messages) != 3 {
		t.Fatalf("want 3 messages, got %d", len(messages))
	}
	blocks := messages[0].(map[string]any)["content"].([]any)
	source := blocks[0].(map[string]any)["source"].(map[string]any)
	if blocks[0].(map[string]any)["type"] != "image" || source["type"] != "base64" || source["media_type"] != "image/jpeg" {
		t.Fatalf("image block wrong: %v", blocks[0])
	}
	if source["data"] != base64.StdEncoding.EncodeToString(test_jpeg) {
		t.Fatalf("image data wrong")
	}
	if blocks[1].(map[string]any)["text"] != "describe" {
		t.Fatalf("text block should follow the image")
	}
}

func TestGeminiRequestFormat(t *testing.T) {
	server, captured := capture_server(t, http.StatusOK, `{"candidates":[{"content":{"parts":[{"text":"{\"folders\":[]}"}]}}]}`)
	completer, err := NewCompleter(ModelRuntime{
		Profile: ModelProfile{Provider: ProviderGemini, BaseURL: server.URL, Model: "models/gemini-test"},
		APIKey:  "g-key",
	}, nil)
	if err != nil {
		t.Fatalf("new completer: %v", err)
	}
	reply, err := completer.Complete(context.Background(), test_request())
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if reply.Text != `{"folders":[]}` {
		t.Fatalf("reply = %q", reply.Text)
	}
	if captured.path != "/models/gemini-test:generateContent" {
		t.Fatalf("path = %s", captured.path)
	}
	if captured.headers.Get("x-goog-api-key") != "g-key" {
		t.Fatalf("x-goog-api-key header missing")
	}
	body := captured.json_body(t)
	contents := body["contents"].([]any)
	if contents[1].(map[string]any)["role"] != "model" {
		t.Fatalf("assistant turn must map to role model")
	}
	first := contents[0].(map[string]any)["parts"].([]any)
	inline := first[0].(map[string]any)["inlineData"].(map[string]any)
	if inline["mimeType"] != "image/jpeg" || inline["data"] != base64.StdEncoding.EncodeToString(test_jpeg) {
		t.Fatalf("inlineData wrong: %v", inline)
	}
	if body["systemInstruction"].(map[string]any)["parts"].([]any)[0].(map[string]any)["text"] != "be brief" {
		t.Fatalf("systemInstruction wrong")
	}
	gen := body["generationConfig"].(map[string]any)
	if gen["responseMimeType"] != "application/json" {
		t.Fatalf("JSON output should set responseMimeType")
	}
}

func TestProviderErrorsAreClear(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   string
		is     error
	}{
		{"unauthorized", http.StatusUnauthorized, `{"error":{"message":"Incorrect API key provided"}}`, "invalid API key", ErrInvalidAPIKey},
		{"forbidden", http.StatusForbidden, `{}`, "invalid API key", ErrInvalidAPIKey},
		{"rate limit", http.StatusTooManyRequests, `{}`, "rate limited", ErrRateLimited},
		{"not found", http.StatusNotFound, `{}`, "model or endpoint not found", nil},
		{"server", http.StatusInternalServerError, `{}`, "server error", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server, _ := capture_server(t, tc.status, tc.body)
			completer, err := NewCompleter(ModelRuntime{
				Profile: ModelProfile{Provider: ProviderAnthropic, BaseURL: server.URL, Model: "m"},
				APIKey:  "leak-canary-998",
			}, nil)
			if err != nil {
				t.Fatalf("new completer: %v", err)
			}
			_, err = completer.Complete(context.Background(), Request{Turns: []Turn{{Role: "user", Text: "x"}}})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want it to mention %q", err, tc.want)
			}
			if tc.is != nil && !errors.Is(err, tc.is) {
				t.Fatalf("error should wrap %v, got %v", tc.is, err)
			}
			if strings.Contains(err.Error(), "leak-canary-998") {
				t.Fatalf("error must not leak the API key")
			}
		})
	}
}

func TestProviderEmptyReplyIsError(t *testing.T) {
	server, _ := capture_server(t, http.StatusOK, `{"choices":[{"message":{"content":"   "}}]}`)
	completer, _ := NewCompleter(ModelRuntime{Profile: ModelProfile{Provider: ProviderOpenAI, BaseURL: server.URL, Model: "m"}, APIKey: "k"}, nil)
	if _, err := completer.Complete(context.Background(), Request{Turns: []Turn{{Role: "user", Text: "x"}}}); err == nil {
		t.Fatalf("blank reply should be an error")
	}
}

func TestProviderHonorsCancellation(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	t.Cleanup(func() {
		close(release)
		server.Close()
	})
	ctx, cancel := context.WithCancel(context.Background())
	completer, _ := NewCompleter(ModelRuntime{Profile: ModelProfile{Provider: ProviderOpenAI, BaseURL: server.URL, Model: "m"}, APIKey: "k"}, nil)
	cancel()
	if _, err := completer.Complete(ctx, Request{Turns: []Turn{{Role: "user", Text: "x"}}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled context should return context.Canceled, got %v", err)
	}
}

func TestNewCompleterRejectsCustomWithoutURL(t *testing.T) {
	if _, err := NewCompleter(ModelRuntime{Profile: ModelProfile{Provider: ProviderCustom, Model: "m"}}, nil); err == nil {
		t.Fatalf("custom provider without base URL must fail")
	}
}
