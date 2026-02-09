package openairesponse

import "github.com/cloudwego/eino/schema"

// toResponsesToolChoice converts Eino's ToolChoice into Responses API tool_choice.
//
// For now we only use the string modes ("auto", "none", "required").
// If you need "force a specific tool", you can pass a single allowed tool name
// and ToolChoiceForced; we will still send "required" and filter tools to that one.
func toResponsesToolChoice(choice schema.ToolChoice, _ []string) any {
	switch choice {
	case schema.ToolChoiceForbidden:
		return "none"
	case schema.ToolChoiceAllowed:
		return "auto"
	case schema.ToolChoiceForced:
		return "required"
	default:
		return nil
	}
}
