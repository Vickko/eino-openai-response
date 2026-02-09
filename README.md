# eino-openai-response

An [Eino](https://github.com/cloudwego/eino) `ChatModel` implementation for the [OpenAI Responses API](https://platform.openai.com/docs/api-reference/responses).

## Background

The Eino framework currently does not have built-in support for OpenAI's Responses API (see [cloudwego/eino#461](https://github.com/cloudwego/eino/issues/461)). The maintainers are still discussing how to properly support it, as the Responses API differs significantly from the Chat Completions API in its data model and capabilities (e.g., `reasoning.summary`, `previous_response_id`, multi-turn via server-side state, etc.).

This package provides a working implementation as an interim solution. It is not a comprehensive or fully-featured client — it covers the common use cases (text generation, streaming, reasoning configuration, multi-modal input, function calling) and can serve as a drop-in `model.ChatModel` within Eino pipelines.

## Install

```bash
go get github.com/Vickko/eino-openai-response
```

## Usage

```go
package main

import (
    "context"
    "errors"
    "fmt"
    "io"

    openairesponse "github.com/Vickko/eino-openai-response"
    "github.com/cloudwego/eino/components/model"
    "github.com/cloudwego/eino/schema"
)

func main() {
    ctx := context.Background()

    client, err := openairesponse.NewChatModel(ctx, &openairesponse.Config{
        APIKey:  "sk-xxx",
        BaseURL: "https://api.openai.com/v1", // optional
        Model:   "gpt-4o",
    })
    if err != nil {
        panic(err)
    }

    // Synchronous generation
    msg, err := client.Generate(ctx, []*schema.Message{
        {Role: schema.User, Content: "Hello!"},
    })
    if err != nil {
        panic(err)
    }
    fmt.Println(msg.Content)
}
```

### Streaming

```go
stream, err := client.Stream(ctx, messages)
if err != nil {
    panic(err)
}
defer stream.Close()

for {
    msg, err := stream.Recv()
    if errors.Is(err, io.EOF) {
        break
    }
    if err != nil {
        panic(err)
    }
    // Some chunks are "meta" only (for example, the last chunk may only carry usage).
    // So always check fields before using them.
    if msg.Content != "" {
        fmt.Print(msg.Content)
    }
    if msg.ResponseMeta != nil && msg.ResponseMeta.Usage != nil {
        // You can read token usage here.
        _ = msg.ResponseMeta.Usage.TotalTokens
    }
}
```

### Reasoning (o-series / gpt-5+)

```go
client, _ := openairesponse.NewChatModel(ctx, &openairesponse.Config{
    APIKey: "sk-xxx",
    Model:  "o3",
})

msg, err := client.Generate(ctx, messages,
    openairesponse.WithReasoningEffort(openairesponse.ReasoningEffortHigh),
    openairesponse.WithReasoningSummary(openairesponse.ReasoningSummaryDetailed),
)
// msg.ReasoningContent contains the reasoning summary
```

### Tool Calling (Function Tools)

You can bind tools on the client, and they will be included in requests.

```go
toolWeather := &schema.ToolInfo{
    Name: "get_weather",
    Desc: "Get the weather for a city",
    ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
        "city": {Type: schema.String, Desc: "City name", Required: true},
    }),
}

// Bind once (mutates the client).
_ = client.BindTools([]*schema.ToolInfo{toolWeather})

// Or create a new client with tools (does not mutate the original client).
clientWithTools, _ := client.WithTools([]*schema.ToolInfo{toolWeather})

// Per-call tools override the bound tools.
_, _ = clientWithTools.Generate(ctx, []*schema.Message{
    schema.UserMessage("What is the weather in Seattle?"),
}, model.WithTools([]*schema.ToolInfo{toolWeather}))
```

Notes:
- `BindTools` / `WithTools` will return an error if you pass an empty tool list.
- If you force tool calling (`ToolChoiceForced`) but no tools are available (for example, all tools were filtered out),
  this client will return an error before making the API request.

### Multi-Turn With Server-Side State (Optional)

If you enable `store`, this client will save the Responses API `response_id` into `schema.Message.Extra`.
On the next turn, if you pass the previous assistant message back in `messages`, the client will
automatically set `previous_response_id` and only send the incremental inputs.

You can also read the id via `openairesponse.GetResponseID(msg)`.

Example:

```go
// First turn (store enabled)
out1, _ := client.Generate(ctx, []*schema.Message{
    schema.UserMessage("Remember that my favorite color is blue."),
}, openairesponse.WithStore(true))

// Second turn: pass the previous assistant message back.
// The client will automatically:
// - set previous_response_id to out1's response_id
// - only send the new user message to the API
out2, _ := client.Generate(ctx, []*schema.Message{
    schema.UserMessage("Remember that my favorite color is blue."),
    out1,
    schema.UserMessage("What is my favorite color?"),
}, openairesponse.WithStore(true))

_ = out2
```

Important details:
- Automatic `previous_response_id` only kicks in when `store=true` (from `WithStore(true)` or `Config.Store=true`),
  and only when the request did not already set `WithPreviousResponseID(...)`.
- If the client cannot find any incremental input after the last stored `response_id`, it will return an error.
- You can still send tools and `tool_choice` when `previous_response_id` is set.

## Features

- Implements `model.ChatModel` interface from Eino
- Synchronous and streaming generation
- System message → `instructions` conversion
- Multi-modal input (text, image, file)
- Function calling / tool use
- Reasoning configuration (`effort` + `summary`)
- Token usage reporting
- Stores `response_id` in `schema.Message.Extra` for multi-turn via server-side state
- Eino callbacks integration

## Things To Know (Limitations)

- `Stop` is not supported by this client for Responses API requests.
- This is not a full Responses API implementation. It focuses on common chat use cases.

## Running Tests

```bash
export OPENAI_API_KEY=sk-xxx
export OPENAI_BASE_URL=https://api.openai.com/v1  # optional
go test -v ./...

# Integration tests (real API calls)
go test -tags=integration -v ./...
```

## License

Apache License 2.0
