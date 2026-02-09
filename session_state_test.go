package openairesponse

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestResponseID_ConcatDoesNotConcatenateStrings(t *testing.T) {
	m1 := &schema.Message{Role: schema.Assistant, Content: "a"}
	setResponseID(m1, "resp_1")

	m2 := &schema.Message{Role: schema.Assistant, Content: "b"}
	setResponseID(m2, "resp_1")

	merged, err := schema.ConcatMessages([]*schema.Message{m1, m2})
	if err != nil {
		t.Fatalf("ConcatMessages: %v", err)
	}

	id, ok := GetResponseID(merged)
	if !ok {
		t.Fatalf("expected response id in merged message, got none: %+v", merged.Extra)
	}
	if id != "resp_1" {
		t.Fatalf("expected resp_1, got %q", id)
	}
}

func TestGenerate_AutoPreviousResponseID_WhenStoreEnabled(t *testing.T) {
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

	// history: user -> assistant(with response_id) -> user(new turn)
	h1 := &schema.Message{Role: schema.User, Content: "hello"}
	a1 := &schema.Message{Role: schema.Assistant, Content: "hi"}
	setResponseID(a1, "resp_prev")
	h2 := &schema.Message{Role: schema.User, Content: "next"}

	_, err = client.Generate(context.Background(), []*schema.Message{h1, a1, h2}, WithStore(true))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if got["previous_response_id"] != "resp_prev" {
		t.Fatalf("expected previous_response_id=resp_prev, got: %v", got["previous_response_id"])
	}
	if got["tools"] != nil {
		t.Fatalf("expected tools to be omitted when previous_response_id is set, got: %v", got["tools"])
	}

	// input should only contain the incremental message ("next")
	input, ok := got["input"].([]any)
	if !ok || len(input) != 1 {
		t.Fatalf("expected exactly 1 input item, got: %T %v", got["input"], got["input"])
	}
	item, _ := input[0].(map[string]any)
	if item["role"] != "user" {
		t.Fatalf("expected role=user, got: %v", item["role"])
	}
}
