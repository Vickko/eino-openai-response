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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

const (
	responsesEndpoint = "/responses"
)

// Client OpenAI Responses API 客户端
type Client struct {
	config *Config

	// rawTools/tools are the tools bound to this model instance via BindTools/WithTools.
	// They are used when caller does not pass model.WithTools(...) in per-call options.
	rawTools []*schema.ToolInfo
	tools    []FunctionTool

	// defaultToolChoice is used when tools are bound and caller does not pass model.WithToolChoice(...).
	defaultToolChoice *schema.ToolChoice
}

// NewChatModel 创建 Responses API 客户端
func NewChatModel(ctx context.Context, config *Config) (*Client, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if config.APIKey == "" {
		return nil, fmt.Errorf("api_key is required")
	}
	if config.Model == "" {
		return nil, fmt.Errorf("model is required")
	}

	return &Client{
		config: config,
	}, nil
}

// Generate 生成响应 (同步)
func (c *Client) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	// Common options (shared across models in Eino)
	commonOpts := model.GetCommonOptions(&model.Options{
		Temperature: c.config.Temperature,
		MaxTokens:   c.config.MaxOutputTokens,
		Model:       &c.config.Model,
		TopP:        c.config.TopP,
	}, opts...)

	// Implementation-specific options
	options := getOptions(c.config, opts)

	// 构建请求
	req, toolInfos, toolChoice, err := c.buildRequest(messages, commonOpts, options, false)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	// 回调 OnStart
	ctx = callbacks.OnStart(ctx, &model.CallbackInput{
		Messages:   messages,
		Tools:      toolInfos,
		ToolChoice: toolChoice,
		Config: &model.Config{
			Model: req.Model,
		},
	})

	// 发送请求
	resp, err := c.doRequest(ctx, req)
	if err != nil {
		_ = callbacks.OnError(ctx, err)
		return nil, err
	}

	// 解析响应
	var response ResponsesResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		resp.Body.Close()
		_ = callbacks.OnError(ctx, err)
		return nil, fmt.Errorf("decode response: %w", err)
	}
	resp.Body.Close()

	// 检查错误
	if response.Error != nil {
		err := fmt.Errorf("api error: %s", response.Error.Message)
		_ = callbacks.OnError(ctx, err)
		return nil, err
	}

	// 转换为 schema.Message
	msg := toSchemaMessage(response.Output, response.Usage)
	setResponseID(msg, response.ID)

	// 回调 OnEnd
	_ = callbacks.OnEnd(ctx, &model.CallbackOutput{
		Message:    msg,
		Config:     &model.Config{Model: req.Model},
		TokenUsage: toModelTokenUsage(response.Usage),
	})

	return msg, nil
}

// Stream 流式生成
func (c *Client) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	commonOpts := model.GetCommonOptions(&model.Options{
		Temperature: c.config.Temperature,
		MaxTokens:   c.config.MaxOutputTokens,
		Model:       &c.config.Model,
		TopP:        c.config.TopP,
	}, opts...)

	options := getOptions(c.config, opts)

	// 构建请求
	req, toolInfos, toolChoice, err := c.buildRequest(messages, commonOpts, options, true)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	cbInput := &model.CallbackInput{
		Messages:   messages,
		Tools:      toolInfos,
		ToolChoice: toolChoice,
		Config: &model.Config{
			Model: req.Model,
		},
	}

	// 回调 OnStart
	ctx = callbacks.OnStart(ctx, cbInput)

	// 发送请求
	resp, err := c.doRequest(ctx, req)
	if err != nil {
		_ = callbacks.OnError(ctx, err)
		return nil, err
	}

	// 检查响应类型
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/event-stream") {
		// 非流式响应，可能是错误
		var errResp ResponsesResponse
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil && errResp.Error != nil {
			resp.Body.Close()
			err := fmt.Errorf("api error: %s", errResp.Error.Message)
			_ = callbacks.OnError(ctx, err)
			return nil, err
		}
		resp.Body.Close()
		err := fmt.Errorf("unexpected content type: %s", contentType)
		_ = callbacks.OnError(ctx, err)
		return nil, err
	}

	// 创建流读取器
	reader := newStreamReader(resp.Body)

	// 创建 Pipe
	sr, sw := schema.Pipe[*model.CallbackOutput](1)

	// 用于通知读取 goroutine 已退出
	readDone := make(chan struct{})

	// 监听 context 取消，主动关闭连接
	go func() {
		select {
		case <-ctx.Done():
			// context 被取消，关闭连接以中断读取
			resp.Body.Close()
		case <-readDone:
			// 读取正常结束，无需处理
		}
	}()

	// 启动 goroutine 读取流
	go func() {
		defer func() {
			close(readDone) // 通知 context 监听 goroutine 退出
			_ = reader.Close()
			resp.Body.Close()
			sw.Close()
		}()

		for {
			msg, recvErr := reader.Recv()
			if recvErr != nil {
				if recvErr == io.EOF {
					// 正常结束
					return
				}
				// context 取消导致的错误不需要发送给下游
				if ctx.Err() != nil {
					return
				}
				// 发送错误
				_ = sw.Send(nil, recvErr)
				return
			}

			if msg != nil {
				var tokenUsage *model.TokenUsage
				if msg.ResponseMeta != nil && msg.ResponseMeta.Usage != nil {
					tokenUsage = toModelTokenUsageFromSchema(msg.ResponseMeta.Usage)
				}
				closed := sw.Send(&model.CallbackOutput{
					Message:    msg,
					Config:     cbInput.Config,
					TokenUsage: tokenUsage,
				}, nil)
				if closed {
					return
				}
			}
		}
	}()

	// 使用回调包装
	ctx, nsr := callbacks.OnEndWithStreamOutput(ctx, schema.StreamReaderWithConvert(sr,
		func(src *model.CallbackOutput) (callbacks.CallbackOutput, error) {
			return src, nil
		}))

	// 转换为消息流
	outStream := schema.StreamReaderWithConvert(nsr,
		func(src callbacks.CallbackOutput) (*schema.Message, error) {
			s := src.(*model.CallbackOutput)
			if s.Message == nil {
				return nil, schema.ErrNoValue
			}
			return s.Message, nil
		},
	)

	return outStream, nil
}

// buildRequest 构建请求
func (c *Client) buildRequest(messages []*schema.Message, commonOpts *model.Options, opts *responsesOptions, stream bool) (*ResponsesRequest, []*schema.ToolInfo, *schema.ToolChoice, error) {
	if commonOpts == nil {
		commonOpts = &model.Options{}
	}
	if len(commonOpts.Stop) > 0 {
		return nil, nil, nil, fmt.Errorf("stop is not supported by Responses API")
	}

	// If store is enabled, we can try to reuse server-side state via previous_response_id
	// by extracting response_id from history messages and only sending incremental inputs.
	messagesForRequest := messages
	if opts.PreviousResponseID == "" {
		store := opts.Store
		if store == nil {
			store = c.config.Store
		}
		if store != nil && *store {
			if id, idx, ok := findLastResponseID(messages); ok {
				if idx+1 >= len(messages) {
					return nil, nil, nil, fmt.Errorf("not found incremental input after response id")
				}
				opts.PreviousResponseID = id
				messagesForRequest = messages[idx+1:]
			}
		}
	}

	// 转换消息
	input, instructions, err := toResponsesInput(messagesForRequest)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("convert messages: %w", err)
	}

	// 使用选项中的 instructions 覆盖
	if opts.Instructions != "" {
		instructions = opts.Instructions
	}

	modelName := c.config.Model
	if commonOpts.Model != nil && *commonOpts.Model != "" {
		modelName = *commonOpts.Model
	}

	req := &ResponsesRequest{
		Model:        modelName,
		Stream:       stream,
		Instructions: instructions,
	}

	// 设置 input
	if len(input) > 0 {
		req.Input = input
	}

	// 设置 reasoning 配置
	if opts.ReasoningEffort != "" || opts.ReasoningSummary != "" {
		req.Reasoning = &ReasoningConfig{
			Effort:  string(opts.ReasoningEffort),
			Summary: string(opts.ReasoningSummary),
		}
	}

	// max tokens: prefer common option
	if commonOpts.MaxTokens != nil {
		req.MaxOutputTokens = commonOpts.MaxTokens
	} else if opts.MaxOutputTokens != nil {
		req.MaxOutputTokens = opts.MaxOutputTokens
	}

	// temperature: prefer common option
	if commonOpts.Temperature != nil {
		temp := float64(*commonOpts.Temperature)
		req.Temperature = &temp
	} else if opts.Temperature != nil {
		temp := float64(*opts.Temperature)
		req.Temperature = &temp
	}

	// top_p: prefer common option
	if commonOpts.TopP != nil {
		topP := float64(*commonOpts.TopP)
		req.TopP = &topP
	} else if opts.TopP != nil {
		topP := float64(*opts.TopP)
		req.TopP = &topP
	}
	if opts.Store != nil {
		req.Store = opts.Store
	}
	if opts.PreviousResponseID != "" {
		req.PreviousResponseID = opts.PreviousResponseID
		// If user is using server-side state, we should store this response by default
		// so it can be referenced in the next turn.
		if req.Store == nil {
			storeTrue := true
			req.Store = &storeTrue
		}
	}

	// Resolve tools for this request (model.WithTools overrides bound tools).
	toolInfos := c.rawTools
	if commonOpts.Tools != nil {
		toolInfos = commonOpts.Tools
	}
	if len(commonOpts.AllowedToolNames) > 0 && len(toolInfos) > 0 {
		allowed := make(map[string]struct{}, len(commonOpts.AllowedToolNames))
		for _, n := range commonOpts.AllowedToolNames {
			allowed[n] = struct{}{}
		}
		filtered := make([]*schema.ToolInfo, 0, len(toolInfos))
		for _, ti := range toolInfos {
			if ti == nil {
				continue
			}
			if _, ok := allowed[ti.Name]; ok {
				filtered = append(filtered, ti)
			}
		}
		toolInfos = filtered
	}

	toolChoice := commonOpts.ToolChoice
	if toolChoice == nil {
		toolChoice = c.defaultToolChoice
	}

	// When continuing a server-side session, avoid sending tools/tool_choice again.
	// This matches the conservative behavior in eino-ext/ark.
	if req.PreviousResponseID != "" {
		return req, toolInfos, toolChoice, nil
	}

	if len(toolInfos) > 0 {
		tools, err := toTools(toolInfos)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("convert tools: %w", err)
		}
		req.Tools = tools
	}

	if toolChoice != nil {
		req.ToolChoice = toResponsesToolChoice(*toolChoice, commonOpts.AllowedToolNames)
	}

	return req, toolInfos, toolChoice, nil
}

// doRequest 发送 HTTP 请求
func (c *Client) doRequest(ctx context.Context, req *ResponsesRequest) (*http.Response, error) {
	// 序列化请求
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// 构建 HTTP 请求
	url := c.config.getBaseURL() + responsesEndpoint
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create http request: %w", err)
	}

	// 设置请求头
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	if req.Stream {
		httpReq.Header.Set("Accept", "text/event-stream")
	}

	// 发送请求
	resp, err := c.config.getHTTPClient().Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}

	// 检查 HTTP 状态码
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		bodyBytes, _ := io.ReadAll(resp.Body)

		// 尝试解析错误响应
		var errResp struct {
			Error *ErrorInfo `json:"error"`
		}
		if json.Unmarshal(bodyBytes, &errResp) == nil && errResp.Error != nil {
			return nil, fmt.Errorf("api error (status %d): %s", resp.StatusCode, errResp.Error.Message)
		}

		return nil, fmt.Errorf("http error: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	return resp, nil
}

// BindTools 绑定工具
func (c *Client) BindTools(tools []*schema.ToolInfo) error {
	if len(tools) == 0 {
		return fmt.Errorf("no tools to bind")
	}

	converted, err := toTools(tools)
	if err != nil {
		return err
	}

	tc := schema.ToolChoiceAllowed
	c.rawTools = tools
	c.tools = converted
	c.defaultToolChoice = &tc
	return nil
}

// GetType 获取类型标识
func (c *Client) GetType() string {
	return "OpenAIResponses"
}

// IsCallbacksEnabled 是否启用回调
func (c *Client) IsCallbacksEnabled() bool {
	return true
}

// 确保实现了接口
var _ model.ChatModel = (*Client)(nil)
var _ model.ToolCallingChatModel = (*Client)(nil)

// WithTools returns a new client with tools bound.
// This avoids mutating the existing client and is safer for concurrent use.
func (c *Client) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	if len(tools) == 0 {
		return nil, fmt.Errorf("no tools to bind")
	}
	converted, err := toTools(tools)
	if err != nil {
		return nil, err
	}

	tc := schema.ToolChoiceAllowed

	nc := *c
	nc.rawTools = tools
	nc.tools = converted
	nc.defaultToolChoice = &tc
	return &nc, nil
}
