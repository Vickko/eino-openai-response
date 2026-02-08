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
    "fmt"

    openairesponse "github.com/Vickko/eino-openai-response"
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
    fmt.Print(msg.Content)
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

## Features

- Implements `model.ChatModel` interface from Eino
- Synchronous and streaming generation
- System message → `instructions` conversion
- Multi-modal input (text, image, file)
- Function calling / tool use
- Reasoning configuration (`effort` + `summary`)
- Token usage reporting
- Eino callbacks integration

## Running Tests

```bash
export OPENAI_API_KEY=sk-xxx
export OPENAI_BASE_URL=https://api.openai.com/v1  # optional
go test -v ./...
```

## License

Apache License 2.0
