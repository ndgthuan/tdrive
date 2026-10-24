// Copyright (c) 2026 Nguyễn Dương Gia Thuận

package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// wire_server answers a fixed sequence of raw JSON bodies and records every
// request body it received.
type wire_server struct {
	server  *httptest.Server
	mu      sync.Mutex
	replies []string
	bodies  []map[string]any
}

func new_wire_server(t *testing.T, replies ...string) *wire_server {
	t.Helper()
	ws := &wire_server{replies: replies}
	ws.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		ws.mu.Lock()
		ws.bodies = append(ws.bodies, body)
		index := len(ws.bodies) - 1
		ws.mu.Unlock()
		if index >= len(ws.replies) {
			http.Error(w, `{"error":{"message":"no scripted reply"}}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(ws.replies[index]))
	}))
	t.Cleanup(ws.server.Close)
	return ws
}

func (ws *wire_server) body(t *testing.T, index int) map[string]any {
	t.Helper()
	ws.mu.Lock()
	defer ws.mu.Unlock()
	if index >= len(ws.bodies) {
		t.Fatalf("request %d not made; made %d", index, len(ws.bodies))
	}
	return ws.bodies[index]
}

// as_map and as_list navigate decoded JSON without panicking in tests.
func as_map(t *testing.T, value any) map[string]any {
	t.Helper()
	m, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("want object, got %T: %v", value, value)
	}
	return m
}

func as_list(t *testing.T, value any) []any {
	t.Helper()
	l, ok := value.([]any)
	if !ok {
		t.Fatalf("want array, got %T: %v", value, value)
	}
	return l
}

var probe_image = Image{MIMEType: "image/jpeg", Data: []byte{0xff, 0xd8, 0xff, 0xd9}}

var tool_test_request_tools = []ToolSpec{{
	Name:        "view_media",
	Description: "Look at pictures.",
	Parameters: map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"item_ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
		"required": []string{"item_ids"},
	},
}}

// tool_round_trip is the shared sequence: the model asks for view_media, the
// tool answers with text and one image, and the model replies with text.
func tool_round_trip(t *testing.T, completer Completer, first Reply, second_text string) (Reply, []Turn) {
	t.Helper()
	ctx := context.Background()
	reply, err := completer.Complete(ctx, Request{
		System: "sys",
		Turns:  []Turn{{Role: "user", Text: "look at 1"}},
		Tools:  tool_test_request_tools,
	})
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if len(reply.ToolCalls) != 1 {
		t.Fatalf("want one tool call, got %+v", reply)
	}
	call := reply.ToolCalls[0]
	if call.Name != "view_media" || !strings.Contains(call.Arguments, `"1"`) {
		t.Fatalf("unexpected tool call %+v", call)
	}
	turns := []Turn{
		{Role: "user", Text: "look at 1"},
		{Role: "assistant", ToolCalls: reply.ToolCalls},
		{Role: "user", ToolResults: []ToolResult{{
			CallID:  call.ID,
			Name:    call.Name,
			Content: "1: goku.png (image), shown as image 1",
			Images:  []Image{probe_image},
		}}},
	}
	final, err := completer.Complete(ctx, Request{System: "sys", Turns: turns, Tools: tool_test_request_tools})
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if final.Text != second_text || len(final.ToolCalls) != 0 {
		t.Fatalf("want final text %q, got %+v", second_text, final)
	}
	return reply, turns
}

func TestOpenAIToolCallsAndImageResultsOnTheWire(t *testing.T) {
	ws := new_wire_server(t,
		`{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_a","type":"function","function":{"name":"view_media","arguments":"{\"item_ids\":[\"1\"]}"}}]}}]}`,
		`{"choices":[{"message":{"role":"assistant","content":"It is a dog."}}]}`,
	)
	completer, err := NewCompleter(ModelRuntime{
		Profile: ModelProfile{Provider: ProviderCustom, BaseURL: ws.server.URL + "/v1", Model: "m"},
	}, nil)
	if err != nil {
		t.Fatalf("completer: %v", err)
	}
	tool_round_trip(t, completer, Reply{}, "It is a dog.")

	first := ws.body(t, 0)
	tools := as_list(t, first["tools"])
	fn := as_map(t, as_map(t, tools[0])["function"])
	if fn["name"] != "view_media" || as_map(t, fn["parameters"])["type"] != "object" {
		t.Fatalf("openai tool declaration wrong: %v", fn)
	}

	second := ws.body(t, 1)
	messages := as_list(t, second["messages"])
	assistant := as_map(t, messages[2])
	if assistant["role"] != "assistant" || assistant["content"] != nil {
		t.Fatalf("assistant tool-call message wrong: %v", assistant)
	}
	calls := as_list(t, assistant["tool_calls"])
	if as_map(t, calls[0])["id"] != "call_a" {
		t.Fatalf("tool call id not echoed: %v", calls[0])
	}
	tool_msg := as_map(t, messages[3])
	if tool_msg["role"] != "tool" || tool_msg["tool_call_id"] != "call_a" {
		t.Fatalf("tool result message wrong: %v", tool_msg)
	}
	media := as_map(t, messages[4])
	if media["role"] != "user" {
		t.Fatalf("media should follow the tool result in a user message: %v", media)
	}
	parts := as_list(t, media["content"])
	image := as_map(t, parts[len(parts)-1])
	url := as_map(t, image["image_url"])["url"].(string)
	if image["type"] != "image_url" || !strings.HasPrefix(url, "data:image/jpeg;base64,") {
		t.Fatalf("image part wrong: %v", image)
	}
}

func TestAnthropicToolUseAndImageInToolResult(t *testing.T) {
	ws := new_wire_server(t,
		`{"content":[{"type":"text","text":"Let me look."},{"type":"tool_use","id":"toolu_1","name":"view_media","input":{"item_ids":["1"]}}],"stop_reason":"tool_use"}`,
		`{"content":[{"type":"text","text":"It is a dog."}]}`,
	)
	completer, err := NewCompleter(ModelRuntime{
		Profile: ModelProfile{Provider: ProviderAnthropic, BaseURL: ws.server.URL, Model: "claude-test"},
		APIKey:  "key",
	}, nil)
	if err != nil {
		t.Fatalf("completer: %v", err)
	}
	reply, _ := tool_round_trip(t, completer, Reply{}, "It is a dog.")
	if reply.Text != "Let me look." {
		t.Fatalf("text before tool_use lost: %q", reply.Text)
	}

	first := ws.body(t, 0)
	tool := as_map(t, as_list(t, first["tools"])[0])
	if tool["name"] != "view_media" || as_map(t, tool["input_schema"])["type"] != "object" {
		t.Fatalf("anthropic tool declaration wrong: %v", tool)
	}

	second := ws.body(t, 1)
	messages := as_list(t, second["messages"])
	assistant := as_map(t, messages[1])
	blocks := as_list(t, assistant["content"])
	use := as_map(t, blocks[len(blocks)-1])
	if use["type"] != "tool_use" || use["id"] != "toolu_1" || as_map(t, use["input"])["item_ids"] == nil {
		t.Fatalf("tool_use block wrong: %v", use)
	}
	result_turn := as_map(t, messages[2])
	if result_turn["role"] != "user" {
		t.Fatalf("tool results must be a user turn: %v", result_turn)
	}
	result := as_map(t, as_list(t, result_turn["content"])[0])
	if result["type"] != "tool_result" || result["tool_use_id"] != "toolu_1" {
		t.Fatalf("tool_result block wrong: %v", result)
	}
	inner := as_list(t, result["content"])
	if as_map(t, inner[0])["type"] != "text" {
		t.Fatalf("tool_result text missing: %v", inner)
	}
	image := as_map(t, inner[1])
	source := as_map(t, image["source"])
	if image["type"] != "image" || source["type"] != "base64" || source["media_type"] != "image/jpeg" {
		t.Fatalf("image block inside tool_result wrong: %v", image)
	}
}

func TestAnthropicErrorResultIsFlagged(t *testing.T) {
	ws := new_wire_server(t, `{"content":[{"type":"text","text":"ok"}]}`)
	completer, err := NewCompleter(ModelRuntime{
		Profile: ModelProfile{Provider: ProviderAnthropic, BaseURL: ws.server.URL, Model: "claude-test"},
		APIKey:  "key",
	}, nil)
	if err != nil {
		t.Fatalf("completer: %v", err)
	}
	_, err = completer.Complete(context.Background(), Request{Turns: []Turn{
		{Role: "assistant", ToolCalls: []ToolCall{{ID: "toolu_9", Name: "rename_item", Arguments: `{}`}}},
		{Role: "user", ToolResults: []ToolResult{{CallID: "toolu_9", Name: "rename_item", Content: "denied", IsError: true}}},
	}})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	result := as_map(t, as_list(t, as_map(t, ws.body(t, 0)["messages"].([]any)[1])["content"])[0])
	if result["is_error"] != true {
		t.Fatalf("is_error not set: %v", result)
	}
}

func TestGeminiFunctionCallAndResponseWithImage(t *testing.T) {
	ws := new_wire_server(t,
		`{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"view_media","args":{"item_ids":["1"]}},"thoughtSignature":"sig-1"}]}}]}`,
		`{"candidates":[{"content":{"role":"model","parts":[{"text":"It is a dog."}]}}]}`,
	)
	completer, err := NewCompleter(ModelRuntime{
		Profile: ModelProfile{Provider: ProviderGemini, BaseURL: ws.server.URL, Model: "gemini-test"},
		APIKey:  "key",
	}, nil)
	if err != nil {
		t.Fatalf("completer: %v", err)
	}
	reply, _ := tool_round_trip(t, completer, Reply{}, "It is a dog.")
	if reply.ToolCalls[0].Signature != "sig-1" || reply.ToolCalls[0].ID != "view_media" {
		t.Fatalf("gemini call metadata lost: %+v", reply.ToolCalls[0])
	}

	first := ws.body(t, 0)
	decl := as_map(t, as_list(t, as_map(t, as_list(t, first["tools"])[0])["functionDeclarations"])[0])
	params := as_map(t, decl["parameters"])
	if params["type"] != "OBJECT" {
		t.Fatalf("gemini schema type should be upper-case: %v", params)
	}
	if _, ok := params["additionalProperties"]; ok {
		t.Fatalf("gemini schema must drop additionalProperties: %v", params)
	}
	props := as_map(t, params["properties"])
	if as_map(t, props["item_ids"])["type"] != "ARRAY" {
		t.Fatalf("nested schema types should be upper-case: %v", props)
	}

	second := ws.body(t, 1)
	contents := as_list(t, second["contents"])
	model_turn := as_map(t, contents[1])
	if model_turn["role"] != "model" {
		t.Fatalf("function call should be a model turn: %v", model_turn)
	}
	call_part := as_map(t, as_list(t, model_turn["parts"])[0])
	if as_map(t, call_part["functionCall"])["name"] != "view_media" || call_part["thoughtSignature"] != "sig-1" {
		t.Fatalf("functionCall part or signature wrong: %v", call_part)
	}
	response_turn := as_map(t, contents[2])
	parts := as_list(t, response_turn["parts"])
	response := as_map(t, as_map(t, as_map(t, parts[0])["functionResponse"])["response"])
	if response["result"] == nil {
		t.Fatalf("functionResponse should carry the result: %v", parts[0])
	}
	inline := as_map(t, as_map(t, parts[1])["inlineData"])
	if inline["mimeType"] != "image/jpeg" || inline["data"] == "" {
		t.Fatalf("inlineData image part wrong: %v", parts[1])
	}
}

func TestToolSchemaConversionDropsUnsupportedKeys(t *testing.T) {
	got := gemini_schema(map[string]any{
		"$schema":              "x",
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"type": map[string]any{"type": "string"},
		},
	}).(map[string]any)
	if _, ok := got["$schema"]; ok {
		t.Fatal("$schema should be dropped")
	}
	if got["type"] != "OBJECT" {
		t.Fatalf("type not upper-cased: %v", got["type"])
	}
	props := got["properties"].(map[string]any)
	if _, ok := props["type"].(map[string]any); !ok {
		t.Fatalf("a property named type must survive: %v", props)
	}
}
