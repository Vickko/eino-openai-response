package openairesponse

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

func mustTool(t *testing.T, name string) *schema.ToolInfo {
	t.Helper()
	return &schema.ToolInfo{
		Name: name,
		Desc: "test tool",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"q": {Type: schema.String, Desc: "query", Required: true},
		}),
	}
}

func TestGenerate_RequestIncludesBoundTools(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_1","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}`))
	}))
	defer srv.Close()

	client, err := NewChatModel(context.Background(), &Config{
		APIKey:  "test-key",
		BaseURL: srv.URL,
		Model:   "gpt-4o-mini",
	})
	if err != nil {
		t.Fatalf("NewChatModel: %v", err)
	}

	if err := client.BindTools([]*schema.ToolInfo{mustTool(t, "tool_a")}); err != nil {
		t.Fatalf("BindTools: %v", err)
	}

	_, err = client.Generate(context.Background(), []*schema.Message{
		{Role: schema.User, Content: "hi"},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if got["tools"] == nil {
		t.Fatalf("expected tools in request, got: %v", got)
	}
	if got["tool_choice"] != "auto" {
		t.Fatalf("expected tool_choice=auto, got: %v", got["tool_choice"])
	}
}

func TestGenerate_PerCallToolsOverrideBoundTools(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_1","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`))
	}))
	defer srv.Close()

	client, err := NewChatModel(context.Background(), &Config{
		APIKey:  "test-key",
		BaseURL: srv.URL,
		Model:   "gpt-4o-mini",
	})
	if err != nil {
		t.Fatalf("NewChatModel: %v", err)
	}

	if err := client.BindTools([]*schema.ToolInfo{mustTool(t, "tool_a")}); err != nil {
		t.Fatalf("BindTools: %v", err)
	}

	_, err = client.Generate(context.Background(), []*schema.Message{
		{Role: schema.User, Content: "hi"},
	}, model.WithTools([]*schema.ToolInfo{mustTool(t, "tool_b")}))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	tools, ok := got["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("expected exactly 1 tool, got: %T %v", got["tools"], got["tools"])
	}
	first, _ := tools[0].(map[string]any)
	if first["name"] != "tool_b" {
		t.Fatalf("expected tool_b, got: %v", first["name"])
	}
}

func TestGenerate_AllowedToolNamesFiltersTools(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_1","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`))
	}))
	defer srv.Close()

	client, err := NewChatModel(context.Background(), &Config{
		APIKey:  "test-key",
		BaseURL: srv.URL,
		Model:   "gpt-4o-mini",
	})
	if err != nil {
		t.Fatalf("NewChatModel: %v", err)
	}

	tc, err := client.WithTools([]*schema.ToolInfo{mustTool(t, "tool_a"), mustTool(t, "tool_b")})
	if err != nil {
		t.Fatalf("WithTools: %v", err)
	}

	_, err = tc.Generate(context.Background(), []*schema.Message{
		{Role: schema.User, Content: "hi"},
	}, model.WithToolChoice(schema.ToolChoiceAllowed, "tool_b"))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	tools, ok := got["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("expected exactly 1 tool after filtering, got: %T %v", got["tools"], got["tools"])
	}
	first, _ := tools[0].(map[string]any)
	if first["name"] != "tool_b" {
		t.Fatalf("expected tool_b, got: %v", first["name"])
	}
}

func TestGenerate_IncludesAssistantToolCallsInInput(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_1","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`))
	}))
	defer srv.Close()

	client, err := NewChatModel(context.Background(), &Config{
		APIKey:  "test-key",
		BaseURL: srv.URL,
		Model:   "gpt-4o-mini",
	})
	if err != nil {
		t.Fatalf("NewChatModel: %v", err)
	}

	msgs := []*schema.Message{
		{Role: schema.User, Content: "use tool"},
		{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "tool_a",
						Arguments: `{"q":"x"}`,
					},
				},
			},
		},
		{Role: schema.Tool, ToolCallID: "call_1", Content: `{"result":"y"}`},
		{Role: schema.User, Content: "continue"},
	}

	_, err = client.Generate(context.Background(), msgs)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	input, ok := got["input"].([]any)
	if !ok {
		t.Fatalf("expected input list, got: %T %v", got["input"], got["input"])
	}

	var found bool
	for _, it := range input {
		m, _ := it.(map[string]any)
		if m["type"] == "function_call" {
			if m["call_id"] == "call_1" && m["name"] == "tool_a" && m["arguments"] == `{"q":"x"}` {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("expected a function_call input item, got input: %v", input)
	}
}

func TestGenerate_AllowedToolNamesFilterToEmpty_WithForcedChoiceErrors(t *testing.T) {
	// If AllowedToolNames filters out all tools, tool_choice=required becomes an invalid request.
	// We should fail early instead of sending a bad API request.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server should not be called for an invalid request")
	}))
	defer srv.Close()

	client, err := NewChatModel(context.Background(), &Config{
		APIKey:  "test-key",
		BaseURL: srv.URL,
		Model:   "gpt-4o-mini",
	})
	if err != nil {
		t.Fatalf("NewChatModel: %v", err)
	}

	tc, err := client.WithTools([]*schema.ToolInfo{mustTool(t, "tool_a")})
	if err != nil {
		t.Fatalf("WithTools: %v", err)
	}

	_, err = tc.Generate(context.Background(), []*schema.Message{
		{Role: schema.User, Content: "hi"},
	}, model.WithToolChoice(schema.ToolChoiceForced, "tool_b"))
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestGenerate_PreviousResponseID_IncludesExplicitTools(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_new","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`))
	}))
	defer srv.Close()

	client, err := NewChatModel(context.Background(), &Config{
		APIKey:  "test-key",
		BaseURL: srv.URL,
		Model:   "gpt-4o-mini",
	})
	if err != nil {
		t.Fatalf("NewChatModel: %v", err)
	}

	// Bind a default tool_choice=auto.
	if err := client.BindTools([]*schema.ToolInfo{mustTool(t, "tool_a")}); err != nil {
		t.Fatalf("BindTools: %v", err)
	}

	// history: user -> assistant(with response_id) -> user(new turn)
	h1 := &schema.Message{Role: schema.User, Content: "hello"}
	a1 := &schema.Message{Role: schema.Assistant, Content: "hi"}
	setResponseID(a1, "resp_prev")
	h2 := &schema.Message{Role: schema.User, Content: "next"}

	_, err = client.Generate(context.Background(), []*schema.Message{h1, a1, h2},
		WithStore(true),
		model.WithTools([]*schema.ToolInfo{mustTool(t, "tool_b")}),
	)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if got["previous_response_id"] != "resp_prev" {
		t.Fatalf("expected previous_response_id=resp_prev, got: %v", got["previous_response_id"])
	}
	if got["tool_choice"] != "auto" {
		t.Fatalf("expected tool_choice=auto, got: %v", got["tool_choice"])
	}
	tools, ok := got["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("expected exactly 1 tool, got: %T %v", got["tools"], got["tools"])
	}
	first, _ := tools[0].(map[string]any)
	if first["name"] != "tool_b" {
		t.Fatalf("expected tool_b, got: %v", first["name"])
	}

	// input should only contain the incremental message ("next")
	input, ok := got["input"].([]any)
	if !ok || len(input) != 1 {
		t.Fatalf("expected exactly 1 input item, got: %T %v", got["input"], got["input"])
	}
}

func TestGenerate_AssistantToolCallValidation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server should not be called when input validation fails")
	}))
	defer srv.Close()

	client, err := NewChatModel(context.Background(), &Config{
		APIKey:  "test-key",
		BaseURL: srv.URL,
		Model:   "gpt-4o-mini",
	})
	if err != nil {
		t.Fatalf("NewChatModel: %v", err)
	}

	tests := []struct {
		name string
		msgs []*schema.Message
	}{
		{
			name: "missing call_id",
			msgs: []*schema.Message{
				{Role: schema.User, Content: "hi"},
				{
					Role: schema.Assistant,
					ToolCalls: []schema.ToolCall{{
						ID: "",
						Function: schema.FunctionCall{
							Name:      "tool_a",
							Arguments: `{}`,
						},
					}},
				},
				{Role: schema.User, Content: "continue"},
			},
		},
		{
			name: "missing name",
			msgs: []*schema.Message{
				{Role: schema.User, Content: "hi"},
				{
					Role: schema.Assistant,
					ToolCalls: []schema.ToolCall{{
						ID: "call_1",
						Function: schema.FunctionCall{
							Name:      "",
							Arguments: `{}`,
						},
					}},
				},
				{Role: schema.User, Content: "continue"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.Generate(context.Background(), tt.msgs)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
		})
	}
}
