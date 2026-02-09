//go:build integration

package openairesponse

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/cloudwego/eino/schema"
)

var (
	// export OPENAI_API_KEY=sk-xxx
	// export OPENAI_BASE_URL=https://api.openai.com/v1
	testAPIKey  = os.Getenv("OPENAI_API_KEY")
	testBaseURL = getEnvOrDefault("OPENAI_BASE_URL", "https://api.openai.com/v1")
	testModel   = getEnvOrDefault("OPENAI_TEST_MODEL", "gpt-4o-mini")
)

func getEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func skipIfNoAPIKey(t *testing.T) {
	t.Helper()
	if testAPIKey == "" {
		t.Skip("set OPENAI_API_KEY to run integration tests")
	}
}

func TestGenerate(t *testing.T) {
	skipIfNoAPIKey(t)

	ctx := context.Background()

	client, err := NewChatModel(ctx, &Config{
		APIKey:  testAPIKey,
		BaseURL: testBaseURL,
		Model:   testModel,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	messages := []*schema.Message{
		{Role: schema.User, Content: "Hello! Please respond with exactly 'Hi there!'"},
	}

	msg, err := client.Generate(ctx, messages)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if msg.Content == "" {
		t.Fatal("response content should not be empty")
	}
}

func TestGenerateWithSystemMessage(t *testing.T) {
	skipIfNoAPIKey(t)

	ctx := context.Background()

	client, err := NewChatModel(ctx, &Config{
		APIKey:  testAPIKey,
		BaseURL: testBaseURL,
		Model:   testModel,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	messages := []*schema.Message{
		{Role: schema.System, Content: "You are a helpful assistant that always responds in Chinese."},
		{Role: schema.User, Content: "What is 2+2?"},
	}

	msg, err := client.Generate(ctx, messages)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if msg.Content == "" {
		t.Fatal("response content should not be empty")
	}
}

func TestStream(t *testing.T) {
	skipIfNoAPIKey(t)

	ctx := context.Background()

	client, err := NewChatModel(ctx, &Config{
		APIKey:  testAPIKey,
		BaseURL: testBaseURL,
		Model:   testModel,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	messages := []*schema.Message{
		{Role: schema.User, Content: "Count from 1 to 5, one number per line."},
	}

	stream, err := client.Stream(ctx, messages)
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}
	defer stream.Close()

	var fullContent string
	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("stream recv error: %v", err)
		}
		fullContent += msg.Content
	}

	if fullContent == "" {
		t.Fatal("stream content should not be empty")
	}
}

func TestGenerateWithReasoning(t *testing.T) {
	skipIfNoAPIKey(t)

	ctx := context.Background()

	// requires a reasoning-capable model, may not be available.
	reasoningModel := "o1-mini"

	client, err := NewChatModel(ctx, &Config{
		APIKey:  testAPIKey,
		BaseURL: testBaseURL,
		Model:   reasoningModel,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	messages := []*schema.Message{
		{Role: schema.User, Content: "What is the sum of 123 + 456?"},
	}

	_, err = client.Generate(ctx, messages,
		WithReasoningEffort(ReasoningEffortHigh),
		WithReasoningSummary(ReasoningSummaryDetailed),
	)
	if err != nil {
		t.Skipf("reasoning model may not be available: %v", err)
	}
}

func TestMultiTurn(t *testing.T) {
	skipIfNoAPIKey(t)

	ctx := context.Background()

	client, err := NewChatModel(ctx, &Config{
		APIKey:  testAPIKey,
		BaseURL: testBaseURL,
		Model:   testModel,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	messages := []*schema.Message{
		{Role: schema.User, Content: "My name is Alice."},
	}

	msg1, err := client.Generate(ctx, messages)
	if err != nil {
		t.Fatalf("first turn failed: %v", err)
	}

	messages = append(messages, msg1)
	messages = append(messages, &schema.Message{Role: schema.User, Content: "What is my name?"})

	msg2, err := client.Generate(ctx, messages)
	if err != nil {
		t.Fatalf("second turn failed: %v", err)
	}
	if msg2.Content == "" {
		t.Fatal("response should not be empty")
	}
}

func TestOptions(t *testing.T) {
	skipIfNoAPIKey(t)

	ctx := context.Background()

	client, err := NewChatModel(ctx, &Config{
		APIKey:  testAPIKey,
		BaseURL: testBaseURL,
		Model:   testModel,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	messages := []*schema.Message{
		{Role: schema.User, Content: "Say hello."},
	}

	msg, err := client.Generate(ctx, messages,
		WithMaxOutputTokens(50),
		WithTemperature(0.5),
	)
	if err != nil {
		t.Fatalf("Generate with options failed: %v", err)
	}
	if msg.Content == "" {
		t.Fatal("response content should not be empty")
	}
}
