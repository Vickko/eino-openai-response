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
	fn, _ := first["function"].(map[string]any)
	if fn["name"] != "tool_b" {
		t.Fatalf("expected tool_b, got: %v", fn["name"])
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

	// bind two tools
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
	fn, _ := first["function"].(map[string]any)
	if fn["name"] != "tool_b" {
		t.Fatalf("expected tool_b, got: %v", fn["name"])
	}
}
