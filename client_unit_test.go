package openairesponse

import (
	"context"
	"testing"
)

func TestNewChatModel(t *testing.T) {
	ctx := context.Background()

	// ok
	client, err := NewChatModel(ctx, &Config{
		APIKey:  "test-key",
		BaseURL: "http://example.com",
		Model:   "gpt-4o-mini",
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	if client == nil {
		t.Fatal("client should not be nil")
	}

	// missing api key
	_, err = NewChatModel(ctx, &Config{
		BaseURL: "http://example.com",
		Model:   "gpt-4o-mini",
	})
	if err == nil {
		t.Fatal("should fail without APIKey")
	}

	// missing model
	_, err = NewChatModel(ctx, &Config{
		APIKey:  "test-key",
		BaseURL: "http://example.com",
	})
	if err == nil {
		t.Fatal("should fail without Model")
	}
}
