package openairesponse

import (
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// toModelTokenUsage converts Responses API usage to Eino callback token usage.
func toModelTokenUsage(usage *Usage) *model.TokenUsage {
	if usage == nil {
		return nil
	}

	out := &model.TokenUsage{
		PromptTokens:     usage.InputTokens,
		CompletionTokens: usage.OutputTokens,
		TotalTokens:      usage.TotalTokens,
	}

	if usage.InputTokensDetails != nil {
		out.PromptTokenDetails.CachedTokens = usage.InputTokensDetails.CachedTokens
	}
	if usage.OutputTokensDetails != nil {
		out.CompletionTokensDetails.ReasoningTokens = usage.OutputTokensDetails.ReasoningTokens
	}

	return out
}

// toModelTokenUsageFromSchema converts schema.TokenUsage (stored on schema.Message) to callback token usage.
func toModelTokenUsageFromSchema(usage *schema.TokenUsage) *model.TokenUsage {
	if usage == nil {
		return nil
	}

	return &model.TokenUsage{
		PromptTokens: usage.PromptTokens,
		PromptTokenDetails: model.PromptTokenDetails{
			CachedTokens: usage.PromptTokenDetails.CachedTokens,
		},
		CompletionTokens: usage.CompletionTokens,
		CompletionTokensDetails: model.CompletionTokensDetails{
			ReasoningTokens: usage.CompletionTokensDetails.ReasoningTokens,
		},
		TotalTokens: usage.TotalTokens,
	}
}
