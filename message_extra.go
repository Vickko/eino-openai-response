package openairesponse

import (
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

const (
	keyOfResponseID = "openai-response-id"
)

// Use a custom type so stream chunk concat does not concatenate strings.
type openAIResponseID string

func init() {
	compose.RegisterStreamChunkConcatFunc(func(chunks []openAIResponseID) (openAIResponseID, error) {
		if len(chunks) == 0 {
			return "", nil
		}
		// Some chunks may not contain a response ID. Taking the first non-empty is safer.
		for _, c := range chunks {
			if c != "" {
				return c, nil
			}
		}
		return "", nil
	})
	schema.RegisterName[openAIResponseID]("_eino_openai_response_id")
}

// GetResponseID returns the Responses API response id from message.Extra.
func GetResponseID(msg *schema.Message) (string, bool) {
	if msg == nil || msg.Extra == nil {
		return "", false
	}
	if v, ok := msg.Extra[keyOfResponseID].(openAIResponseID); ok && v != "" {
		return string(v), true
	}
	// When users serialize/deserialize schema.Message, the concrete type may be lost.
	if v, ok := msg.Extra[keyOfResponseID].(string); ok && v != "" {
		return v, true
	}
	return "", false
}

func setResponseID(msg *schema.Message, id string) {
	if msg == nil || id == "" {
		return
	}
	if msg.Extra == nil {
		msg.Extra = make(map[string]any)
	}
	msg.Extra[keyOfResponseID] = openAIResponseID(id)
}
