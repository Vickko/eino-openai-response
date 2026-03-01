//go:build integration

package openairesponse

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
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

func weatherTool() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: "get_weather",
		Desc: "Get the current weather for a city",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"city": {Type: schema.String, Desc: "The city name", Required: true},
		}),
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

	// requires a reasoning-capable model
	reasoningModel := "gpt-5"

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

func TestToolCalling(t *testing.T) {
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

	if err := client.BindTools([]*schema.ToolInfo{weatherTool()}); err != nil {
		t.Fatalf("BindTools: %v", err)
	}

	msg, err := client.Generate(ctx, []*schema.Message{
		{Role: schema.User, Content: "What's the weather in Beijing?"},
	})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if len(msg.ToolCalls) == 0 {
		t.Fatalf("expected tool calls, got content: %q", msg.Content)
	}

	tc := msg.ToolCalls[0]
	t.Logf("tool call: id=%s name=%s args=%s", tc.ID, tc.Function.Name, tc.Function.Arguments)

	if tc.Function.Name != "get_weather" {
		t.Fatalf("expected get_weather, got: %s", tc.Function.Name)
	}
	if tc.ID == "" {
		t.Fatal("tool call ID should not be empty")
	}
}

// TestToolCallingMultiTurn tests the full tool calling loop:
// user asks → model calls tool → send tool output → model responds with text
func TestToolCallingMultiTurn(t *testing.T) {
	skipIfNoAPIKey(t)

	ctx := context.Background()

	weatherTools := []*schema.ToolInfo{weatherTool()}

	client, err := NewChatModel(ctx, &Config{
		APIKey:  testAPIKey,
		BaseURL: testBaseURL,
		Model:   testModel,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	if err := client.BindTools(weatherTools); err != nil {
		t.Fatalf("BindTools: %v", err)
	}

	// Turn 1: user asks, model should call tool
	messages := []*schema.Message{
		{Role: schema.User, Content: "What's the weather in Tokyo?"},
	}

	msg1, err := client.Generate(ctx, messages)
	if err != nil {
		t.Fatalf("turn 1 Generate failed: %v", err)
	}
	if len(msg1.ToolCalls) == 0 {
		t.Fatalf("turn 1: expected tool call, got content: %q", msg1.Content)
	}

	tc := msg1.ToolCalls[0]
	t.Logf("turn 1 tool call: id=%s name=%s args=%s", tc.ID, tc.Function.Name, tc.Function.Arguments)

	// Turn 2: send tool output, model should respond with text
	messages = append(messages, msg1)
	messages = append(messages, &schema.Message{
		Role:       schema.Tool,
		ToolCallID: tc.ID,
		Content:    `{"temperature": "22°C", "condition": "sunny"}`,
	})

	msg2, err := client.Generate(ctx, messages)
	if err != nil {
		t.Fatalf("turn 2 Generate failed: %v", err)
	}

	if msg2.Content == "" {
		t.Fatal("turn 2: expected text response, got empty content")
	}
	t.Logf("turn 2 response: %s", msg2.Content)

	// Verify the response mentions the weather data
	lower := strings.ToLower(msg2.Content)
	if !strings.Contains(lower, "22") && !strings.Contains(lower, "sunny") {
		t.Logf("warning: response may not contain weather data: %s", msg2.Content)
	}
}

// TestToolCallingStream tests tool calls via streaming
func TestToolCallingStream(t *testing.T) {
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

	if err := client.BindTools([]*schema.ToolInfo{weatherTool()}); err != nil {
		t.Fatalf("BindTools: %v", err)
	}

	stream, err := client.Stream(ctx, []*schema.Message{
		{Role: schema.User, Content: "What's the weather in London?"},
	})
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}
	defer stream.Close()

	// Collect all tool call chunks
	var (
		callID   string
		funcName string
		argsStr  string
	)

	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("stream recv error: %v", err)
		}

		for _, tc := range msg.ToolCalls {
			if tc.ID != "" {
				callID = tc.ID
			}
			if tc.Function.Name != "" {
				funcName = tc.Function.Name
			}
			argsStr += tc.Function.Arguments
		}
	}

	t.Logf("stream tool call: id=%s name=%s args=%s", callID, funcName, argsStr)

	if funcName != "get_weather" {
		t.Fatalf("expected get_weather, got: %q", funcName)
	}
	if callID == "" {
		t.Fatal("tool call ID should not be empty")
	}
	if argsStr == "" {
		t.Fatal("tool call arguments should not be empty")
	}

	// Verify arguments is valid JSON
	var args map[string]any
	if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
		t.Fatalf("tool call arguments is not valid JSON: %v (raw: %q)", err, argsStr)
	}
	t.Logf("parsed args: %v", args)
}

// TestToolCallingRequired tests tool_choice=required
func TestToolCallingRequired(t *testing.T) {
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

	tools := []*schema.ToolInfo{
		{
			Name: "get_time",
			Desc: "Get the current time in a timezone",
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"timezone": {Type: schema.String, Desc: "The timezone", Required: true},
			}),
		},
	}

	// Use tool_choice=required: model MUST call a tool even for a greeting
	msg, err := client.Generate(ctx, []*schema.Message{
		{Role: schema.User, Content: "Hello!"},
	}, model.WithTools(tools), model.WithToolChoice(schema.ToolChoiceForced))
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if len(msg.ToolCalls) == 0 {
		t.Fatalf("expected tool call with tool_choice=required, got content: %q", msg.Content)
	}

	tc := msg.ToolCalls[0]
	t.Logf("forced tool call: id=%s name=%s args=%s", tc.ID, tc.Function.Name, tc.Function.Arguments)

	if tc.Function.Name != "get_time" {
		t.Fatalf("expected get_time, got: %s", tc.Function.Name)
	}
}
