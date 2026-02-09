/*
 * Copyright 2024 DevOps Backend Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package openairesponse

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/cloudwego/eino/schema"
)

// streamReader SSE 流读取器
type streamReader struct {
	reader   *bufio.Reader
	closer   io.Closer
	response *ResponsesResponse
	err      error

	done bool
}

// newStreamReader 创建流读取器
func newStreamReader(body io.ReadCloser) *streamReader {
	return &streamReader{
		reader: bufio.NewReader(body),
		closer: body,
	}
}

// Close 关闭流
func (s *streamReader) Close() error {
	return s.closer.Close()
}

// Recv 接收下一条消息
// 返回增量消息，当流结束时返回 io.EOF
func (s *streamReader) Recv() (*schema.Message, error) {
	if s.done {
		return nil, io.EOF
	}

	for {
		eventType, data, err := s.readSSEEvent()
		if err != nil {
			if err == io.EOF {
				s.done = true
				return nil, io.EOF
			}
			return nil, err
		}
		if data == "[DONE]" {
			s.done = true
			return nil, io.EOF
		}

		// Some providers may send events without an explicit "event:" line.
		if eventType == "" {
			continue
		}

		msg, done, err := s.handleEvent(eventType, data)
		if err != nil {
			s.done = done
			return nil, err
		}

		if msg != nil && s.response != nil && s.response.ID != "" {
			setResponseID(msg, s.response.ID)
		}

		if done {
			s.done = true
			if msg != nil {
				return msg, nil
			}
			return nil, io.EOF
		}
		if msg != nil {
			return msg, nil
		}
	}
}

// readSSEEvent reads one SSE event and returns (eventType, data).
// It supports multi-line data blocks and ignores comments.
func (s *streamReader) readSSEEvent() (string, string, error) {
	var (
		eventType string
		dataLines []string
	)

	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			// handle last line without trailing '\n'
			if err == io.EOF && len(line) > 0 {
				line = strings.TrimRight(line, "\r\n")
			} else if err == io.EOF {
				// Stream may end without the trailing blank line. If we already
				// collected something, flush it as the last event.
				if eventType != "" || len(dataLines) > 0 {
					return eventType, strings.Join(dataLines, "\n"), nil
				}
				return "", "", io.EOF
			} else {
				return "", "", fmt.Errorf("read stream: %w", err)
			}
		} else {
			line = strings.TrimRight(line, "\r\n")
		}

		// event ends with a blank line
		if line == "" {
			if eventType == "" && len(dataLines) == 0 {
				// skip extra blank lines
				continue
			}
			return eventType, strings.Join(dataLines, "\n"), nil
		}

		// comment
		if strings.HasPrefix(line, ":") {
			continue
		}

		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}

		if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			dataLines = append(dataLines, data)
			continue
		}

		// ignore other fields: id:, retry:, etc.
	}
}

// handleEvent 处理 SSE 事件
func (s *streamReader) handleEvent(eventType, data string) (*schema.Message, bool, error) {
	switch eventType {
	case "response.created":
		var event StreamResponseCreated
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return nil, false, fmt.Errorf("unmarshal response.created: %w", err)
		}
		s.response = event.Response
		return nil, false, nil

	case "response.output_text.delta":
		var event StreamOutputTextDelta
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return nil, false, fmt.Errorf("unmarshal output_text.delta: %w", err)
		}
		if event.Delta != "" {
			return &schema.Message{
				Role:    schema.Assistant,
				Content: event.Delta,
			}, false, nil
		}
		return nil, false, nil

	case "response.reasoning_summary_text.delta":
		var event StreamReasoningSummaryTextDelta
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return nil, false, fmt.Errorf("unmarshal reasoning_summary_text.delta: %w", err)
		}
		if event.Delta != "" {
			return &schema.Message{
				Role:             schema.Assistant,
				ReasoningContent: event.Delta,
			}, false, nil
		}
		return nil, false, nil

	case "response.function_call_arguments.delta":
		var event StreamFunctionCallArgumentsDelta
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return nil, false, fmt.Errorf("unmarshal function_call_arguments.delta: %w", err)
		}
		if event.Delta != "" {
			idx := event.OutputIndex
			return &schema.Message{
				Role: schema.Assistant,
				ToolCalls: []schema.ToolCall{
					{
						Index: &idx,
						ID:    event.CallID,
						Type:  "function",
						Function: schema.FunctionCall{
							Arguments: event.Delta,
						},
					},
				},
			}, false, nil
		}
		return nil, false, nil

	case "response.output_item.done":
		var event StreamOutputItemDone
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return nil, false, fmt.Errorf("unmarshal output_item.done: %w", err)
		}
		// 如果是 function_call 完成，发送完整的工具调用
		if event.Item != nil && event.Item.Type == "function_call" {
			return &schema.Message{
				Role: schema.Assistant,
				ToolCalls: []schema.ToolCall{
					{
						ID:   event.Item.CallID,
						Type: "function",
						Function: schema.FunctionCall{
							Name:      event.Item.Name,
							Arguments: event.Item.Arguments,
						},
					},
				},
			}, false, nil
		}
		return nil, false, nil

	case "response.completed":
		var event StreamResponseCompleted
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return nil, false, fmt.Errorf("unmarshal response.completed: %w", err)
		}
		s.response = event.Response
		// 发送最终消息带 usage
		if event.Response != nil && event.Response.Usage != nil {
			return &schema.Message{
				Role: schema.Assistant,
				ResponseMeta: &schema.ResponseMeta{
					FinishReason: event.Response.Status,
					Usage:        toSchemaTokenUsage(event.Response.Usage),
				},
			}, true, nil
		}
		return nil, true, nil

	case "response.failed":
		var event StreamResponseFailed
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return nil, false, fmt.Errorf("unmarshal response.failed: %w", err)
		}
		if event.Response != nil && event.Response.Error != nil {
			return nil, true, fmt.Errorf("response failed: %s", event.Response.Error.Message)
		}
		return nil, true, fmt.Errorf("response failed")

	case "error":
		var event StreamError
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return nil, true, fmt.Errorf("unmarshal error: %w", err)
		}
		return nil, true, fmt.Errorf("stream error: %s", event.Message)

	case "response.in_progress", "response.output_item.added", "response.content_part.added",
		"response.output_text.done", "response.reasoning_summary_text.done",
		"response.content_part.done":
		// 这些事件不需要处理或只用于状态跟踪
		return nil, false, nil

	default:
		// 忽略未知事件
		return nil, false, nil
	}
}

// GetResponse 获取完整响应
func (s *streamReader) GetResponse() *ResponsesResponse {
	return s.response
}
