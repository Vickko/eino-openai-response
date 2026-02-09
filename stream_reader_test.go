package openairesponse

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestStreamReader_ReturnsFinalMetaBeforeEOF(t *testing.T) {
	sse := strings.Join([]string{
		"event: response.created",
		`data: {"response":{"id":"resp_1","status":"in_progress"}}`,
		"",
		"event: response.output_text.delta",
		`data: {"output_index":0,"content_index":0,"delta":"he"}`,
		"",
		"event: response.output_text.delta",
		`data: {"output_index":0,"content_index":0,"delta":"llo"}`,
		"",
		"event: response.completed",
		`data: {"response":{"id":"resp_1","status":"completed","usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}}`,
		"",
	}, "\n")

	sr := newStreamReader(io.NopCloser(strings.NewReader(sse)))
	defer sr.Close()

	var chunks []*schema.Message
	for {
		m, err := sr.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Recv: %v", err)
		}
		chunks = append(chunks, m)
	}

	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	if chunks[0].Content != "he" || chunks[1].Content != "llo" {
		t.Fatalf("unexpected content chunks: %q %q", chunks[0].Content, chunks[1].Content)
	}

	last := chunks[2]
	if last.ResponseMeta == nil || last.ResponseMeta.Usage == nil {
		t.Fatalf("expected final chunk to have usage, got: %+v", last.ResponseMeta)
	}

	for i, c := range chunks {
		id, ok := GetResponseID(c)
		if !ok || id != "resp_1" {
			t.Fatalf("chunk %d expected response_id=resp_1, got ok=%v id=%q extra=%v", i, ok, id, c.Extra)
		}
	}

	// A second EOF is fine and should be stable.
	_, err := sr.Recv()
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF after done, got: %v", err)
	}
}
