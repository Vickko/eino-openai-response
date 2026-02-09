package openairesponse

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestClientStream_DeliversFinalUsageChunk(t *testing.T) {
	sse := strings.Join([]string{
		"event: response.created",
		`data: {"response":{"id":"resp_1","status":"in_progress"}}`,
		"",
		"event: response.output_text.delta",
		`data: {"output_index":0,"content_index":0,"delta":"ok"}`,
		"",
		"event: response.completed",
		`data: {"response":{"id":"resp_1","status":"completed","usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}}`,
		"",
	}, "\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(sse))
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

	stream, err := client.Stream(context.Background(), []*schema.Message{
		{Role: schema.User, Content: "hi"},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer stream.Close()

	var chunks []*schema.Message
	for {
		m, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Recv: %v", err)
		}
		chunks = append(chunks, m)
	}

	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks (text + final meta), got %d", len(chunks))
	}
	if chunks[0].Content != "ok" {
		t.Fatalf("expected first chunk content ok, got %q", chunks[0].Content)
	}
	if chunks[1].ResponseMeta == nil || chunks[1].ResponseMeta.Usage == nil {
		t.Fatalf("expected final chunk to contain usage, got: %+v", chunks[1].ResponseMeta)
	}
}
