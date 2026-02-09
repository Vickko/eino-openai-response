package openairesponse

import "github.com/cloudwego/eino/schema"

// findLastResponseID scans messages from the end and returns the last response_id we stored in Extra.
// It returns the id, the index where it was found, and ok=true if found.
func findLastResponseID(messages []*schema.Message) (id string, idx int, ok bool) {
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg == nil {
			continue
		}
		if rid, ok := GetResponseID(msg); ok {
			return rid, i, true
		}
	}
	return "", -1, false
}
