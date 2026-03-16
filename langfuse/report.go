package langfuse

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// requestParams 从 dto.Request 中提取对模型调参和 Agent 评估有用的请求参数。
type requestParams struct {
	Temperature     *float64
	MaxTokens       uint
	TopP            *float64
	ReasoningEffort string
	ResponseFormat  string // "text" | "json_object" | "json_schema" | ""
	HasTools        bool
	ToolCount       int
	HasSystemPrompt bool
	MessageCount    int
}

// extractRequestParams 尝试将 dto.Request 转型为 GeneralOpenAIRequest 并提取关键参数。
// 若转型失败（非 OpenAI 兼容格式），返回空结构体。
func extractRequestParams(req dto.Request) requestParams {
	r, ok := req.(*dto.GeneralOpenAIRequest)
	if !ok || r == nil {
		return requestParams{}
	}
	p := requestParams{
		Temperature:     r.Temperature,
		MaxTokens:       r.GetMaxTokens(),
		TopP:            r.TopP,
		ReasoningEffort: r.ReasoningEffort,
		HasTools:        len(r.Tools) > 0,
		ToolCount:       len(r.Tools),
		MessageCount:    len(r.Messages),
	}
	if r.ResponseFormat != nil {
		p.ResponseFormat = r.ResponseFormat.Type
	}
	for _, msg := range r.Messages {
		if msg.Role == "system" {
			p.HasSystemPrompt = true
			break
		}
	}
	return p
}

// extractInputMessages 从请求中提取 messages 数组并序列化为 JSON 字节切片。
// 只取 messages，不包含 model/stream/temperature 等字段，
// 使 Langfuse 能正确识别 ChatML 格式并按 role 分区渲染。
// 返回 nil 表示非 OpenAI 兼容请求或 messages 为空。
func extractInputMessages(req dto.Request) []byte {
	if req == nil {
		return nil
	}
	r, ok := req.(*dto.GeneralOpenAIRequest)
	if !ok || r == nil || len(r.Messages) == 0 {
		return nil
	}
	b, err := common.Marshal(r.Messages)
	if err != nil {
		return nil
	}
	return b
}

// buildTraceMetadata 构建 Trace 级别的 metadata，聚焦于用户身份与请求上下文。
func buildTraceMetadata(snap *reportSnapshot) map[string]any {
	m := map[string]any{
		// 用户身份（便于用户画像分析）
		"username":    snap.Username,
		"user_email":  snap.UserEmail,
		"user_group":  snap.UserGroup,
		"token_name":  snap.TokenName,
		// 请求路由上下文
		"gateway_group":  snap.UsingGroup,
		"billing_source": snap.BillingSource,
		// 请求特征（快速分类用）
		"is_stream":     snap.IsStream,
		"has_reasoning": snap.ReasoningContent != "",
		"retry_count":   snap.RetryIndex,
	}
	// 去除空字符串字段，保持 Langfuse UI 整洁
	for k, v := range m {
		if s, ok := v.(string); ok && s == "" {
			delete(m, k)
		}
	}
	return m
}

// buildGenerationMetadata 构建 Generation 级别的 metadata，聚焦于模型参数、性能指标和响应特征。
func buildGenerationMetadata(snap *reportSnapshot, endTime time.Time) map[string]any {
	// 性能指标
	var ttftMs, totalLatencyMs int64
	if !snap.FirstResponseTime.IsZero() {
		ttftMs = snap.FirstResponseTime.Sub(snap.StartTime).Milliseconds()
	}
	if !snap.StartTime.IsZero() {
		totalLatencyMs = endTime.Sub(snap.StartTime).Milliseconds()
	}

	m := map[string]any{
		// --- 模型参数（调参核心字段）---
		"temperature":      snap.Temperature,
		"max_tokens":       snap.MaxTokens,
		"top_p":            snap.TopP,
		"reasoning_effort": snap.ReasoningEffort,
		"response_format":  snap.ResponseFormat,
		"has_tools":        snap.HasTools,
		"tool_count":       snap.ToolCount,
		"has_system_prompt": snap.HasSystemPrompt,
		"message_count":    snap.MessageCount,
		// --- 上游路由信息 ---
		"upstream_model":  snap.UpstreamModelName,
		"is_model_mapped": snap.IsModelMapped,
		"provider_url":    snap.ChannelBaseUrl,
		// --- 性能指标（Agent 评估关键）---
		"ttft_ms":          ttftMs,
		"total_latency_ms": totalLatencyMs,
		// --- 响应特征（用于质量评估）---
		"response_length":  len(snap.ResponseContent),
		"reasoning_length": len(snap.ReasoningContent),
		"has_reasoning":    snap.ReasoningContent != "",
		"relay_format":     snap.RelayFormat,
		// --- 可靠性 ---
		"retry_count": snap.RetryIndex,
	}
	// 去除 nil 及无意义的零值字段
	if snap.Temperature == nil {
		delete(m, "temperature")
	}
	if snap.TopP == nil {
		delete(m, "top_p")
	}
	for _, k := range []string{"reasoning_effort", "response_format", "upstream_model", "provider_url", "relay_format"} {
		if s, ok := m[k].(string); ok && s == "" {
			delete(m, k)
		}
	}
	return m
}

// ReportGeneration sends a trace + generation event pair to Langfuse synchronously.
// Safe to call when the client is disabled (no-op).
func ReportGeneration(ctx *gin.Context, info *relaycommon.RelayInfo, usage *dto.Usage) {
	if globalClient == nil {
		return
	}

	now := time.Now()
	requestId := info.RequestId
	if requestId == "" {
		requestId = uuid.New().String()
	}

	username := ctx.GetString("username")
	tokenName := ctx.GetString("token_name")

	reqParams := extractRequestParams(info.Request)

	// 只取 messages 数组作为 input，使 Langfuse 能识别 ChatML role 并分区渲染
	var inputData any
	if r, ok := info.Request.(*dto.GeneralOpenAIRequest); ok && r != nil && len(r.Messages) > 0 {
		inputData = r.Messages
	}

	outputData := buildOutputData(info.ResponseContent, info.ReasoningContent)

	tags := []string{}
	if tokenName != "" {
		tags = append(tags, "token:"+tokenName)
	}
	if info.UsingGroup != "" {
		tags = append(tags, "group:"+info.UsingGroup)
	}

	// 为同步路径构建等价的 snapshot 用于 metadata 生成
	snap := &reportSnapshot{
		Username:          username,
		UserEmail:         info.UserEmail,
		UserGroup:         info.UserGroup,
		TokenName:         tokenName,
		UsingGroup:        info.UsingGroup,
		BillingSource:     info.BillingSource,
		RetryIndex:        info.RetryIndex,
		IsStream:          info.IsStream,
		RelayFormat:       string(info.RelayFormat),
		RelayMode:         info.RelayMode,
		StartTime:         info.StartTime,
		FirstResponseTime: info.FirstResponseTime,
		EndTime:           now,
		ResponseContent:   info.ResponseContent,
		ReasoningContent:  info.ReasoningContent,
		Temperature:       reqParams.Temperature,
		MaxTokens:         reqParams.MaxTokens,
		TopP:              reqParams.TopP,
		ReasoningEffort:   reqParams.ReasoningEffort,
		ResponseFormat:    reqParams.ResponseFormat,
		HasTools:          reqParams.HasTools,
		ToolCount:         reqParams.ToolCount,
		HasSystemPrompt:   reqParams.HasSystemPrompt,
		MessageCount:      reqParams.MessageCount,
	}
	if info.ChannelMeta != nil {
		snap.ChannelId = info.ChannelId
		snap.ChannelType = info.ChannelType
		snap.ChannelBaseUrl = info.ChannelBaseUrl
		snap.UpstreamModelName = info.UpstreamModelName
		snap.IsModelMapped = info.IsModelMapped
	}

	var usageData *UsageData
	if usage != nil {
		usageData = &UsageData{
			Input:  usage.PromptTokens,
			Output: usage.CompletionTokens,
			Total:  usage.TotalTokens,
			Unit:   "TOKENS",
		}
	}

	startTime := info.StartTime
	var completionStartTime *time.Time
	if !info.FirstResponseTime.IsZero() {
		t := info.FirstResponseTime
		completionStartTime = &t
	}

	traceEvent := IngestionEvent{
		ID:        "evt-trace-" + requestId,
		Type:      "trace-create",
		Timestamp: now,
		Body: TraceBody{
			ID:       requestId,
			Name:     info.OriginModelName,
			UserID:   username,
			Input:    inputData,
			Output:   outputData,
			Tags:     tags,
			Metadata: buildTraceMetadata(snap),
		},
	}

	generationEvent := IngestionEvent{
		ID:        "evt-gen-" + requestId,
		Type:      "generation-create",
		Timestamp: now,
		Body: GenerationBody{
			ID:                  "gen-" + requestId,
			TraceID:             requestId,
			Name:                info.RequestURLPath,
			Model:               info.OriginModelName,
			Input:               inputData,
			Output:              outputData,
			Usage:               usageData,
			StartTime:           &startTime,
			EndTime:             &now,
			CompletionStartTime: completionStartTime,
			Metadata:            buildGenerationMetadata(snap, now),
		},
	}

	if common.DebugEnabled {
		common.SysLog(fmt.Sprintf("Langfuse [sync]: requestId=%s, model=%s, user=%s, hasReasoning=%v, inputMsgCount=%d, responseLen=%d",
			requestId, info.OriginModelName, username, info.ReasoningContent != "", reqParams.MessageCount, len(info.ResponseContent)))
	}

	Enqueue([]IngestionEvent{traceEvent, generationEvent})
}

// ReportGenerationAsync is a convenience wrapper that calls ReportGeneration
// in a separate goroutine so the caller never blocks.
func ReportGenerationAsync(ctx *gin.Context, info *relaycommon.RelayInfo, usage *dto.Usage) {
	if globalClient == nil {
		return
	}

	username := ctx.GetString("username")
	tokenName := ctx.GetString("token_name")

	// 提取请求参数（在进入 goroutine 前完成，避免并发读写 info.Request）
	reqParams := extractRequestParams(info.Request)

	snap := &reportSnapshot{
		// 请求基础信息
		RequestId:         info.RequestId,
		OriginModelName:   info.OriginModelName,
		RequestURLPath:    info.RequestURLPath,
		RelayFormat:       string(info.RelayFormat),
		RelayMode:         info.RelayMode,
		StartTime:         info.StartTime,
		FirstResponseTime: info.FirstResponseTime,
		EndTime:           time.Now(), // 响应已完成，记录此刻作为结束时间
		IsStream:          info.IsStream,
		// 用户与认证信息
		Username:      username,
		UserEmail:     info.UserEmail,
		UserGroup:     info.UserGroup,
		UserId:        info.UserId,
		TokenId:       info.TokenId,
		TokenName:     tokenName,
		UsingGroup:    info.UsingGroup,
		BillingSource: info.BillingSource,
		RetryIndex:    info.RetryIndex,
		// 响应内容
		ResponseContent:  info.ResponseContent,
		ReasoningContent: info.ReasoningContent,
		// 请求参数
		Temperature:     reqParams.Temperature,
		MaxTokens:       reqParams.MaxTokens,
		TopP:            reqParams.TopP,
		ReasoningEffort: reqParams.ReasoningEffort,
		ResponseFormat:  reqParams.ResponseFormat,
		HasTools:        reqParams.HasTools,
		ToolCount:       reqParams.ToolCount,
		HasSystemPrompt: reqParams.HasSystemPrompt,
		MessageCount:    reqParams.MessageCount,
	}

	if info.ChannelMeta != nil {
		snap.ChannelId = info.ChannelId
		snap.ChannelType = info.ChannelType
		snap.ChannelBaseUrl = info.ChannelBaseUrl
		snap.UpstreamModelName = info.UpstreamModelName
		snap.IsModelMapped = info.IsModelMapped
	}

	// 在进入 goroutine 前提取 messages（[]byte 可安全跨 goroutine 传递）
	snap.InputMessages = extractInputMessages(info.Request)

	if common.DebugEnabled {
		common.SysLog(fmt.Sprintf("Langfuse [async]: requestId=%s, model=%s, user=%s, hasReasoning=%v, msgCount=%d, responseLen=%d, inputBytes=%d",
			snap.RequestId, snap.OriginModelName, snap.Username, snap.ReasoningContent != "",
			snap.MessageCount, len(snap.ResponseContent), len(snap.InputMessages)))
	}

	var usageCopy *dto.Usage
	if usage != nil {
		u := *usage
		usageCopy = &u
	}

	go reportFromSnapshot(snap, usageCopy)
}

type reportSnapshot struct {
	// 请求基础信息
	RequestId       string
	OriginModelName string
	RequestURLPath  string
	RelayFormat     string
	RelayMode       int
	StartTime       time.Time
	FirstResponseTime time.Time
	EndTime         time.Time // 响应完成时刻，用于计算总延迟
	IsStream        bool

	// 用户与认证信息
	Username      string
	UserEmail     string
	UserGroup     string
	UserId        int
	TokenId       int
	TokenName     string
	UsingGroup    string
	BillingSource string
	RetryIndex    int

	// 渠道路由信息
	ChannelId         int
	ChannelType       int
	ChannelBaseUrl    string
	UpstreamModelName string
	IsModelMapped     bool

	// 请求参数（模型调参关键字段）
	Temperature     *float64
	MaxTokens       uint
	TopP            *float64
	ReasoningEffort string
	ResponseFormat  string
	HasTools        bool
	ToolCount       int
	HasSystemPrompt bool
	MessageCount    int

	// 响应内容
	ResponseContent  string
	ReasoningContent string // 仅包含模型的思考/推理内容（不含正式回答）

	// 预序列化的 messages 数组（仅含消息列表，不含模型参数）
	// 使用 []byte 在 goroutine 间传递，避免并发读取 dto.Request
	InputMessages []byte
}

// buildOutputData 根据是否存在思考内容，构建 Langfuse generation output 字段。
// 若存在思考内容，返回符合 Langfuse ChatML schema 的结构化消息，前端可分区展示思考与回答；
// 否则退回为纯字符串，保持与原有行为兼容。
func buildOutputData(responseContent, reasoningContent string) any {
	if responseContent == "" {
		return nil
	}
	if reasoningContent == "" {
		return responseContent
	}
	return ChatMLAssistantMessage{
		Role:    "assistant",
		Content: responseContent,
		Thinking: []ChatMLThinkingPart{
			{Type: "thinking", Content: reasoningContent},
		},
	}
}

func reportFromSnapshot(snap *reportSnapshot, usage *dto.Usage) {
	now := time.Now()
	requestId := snap.RequestId
	if requestId == "" {
		requestId = uuid.New().String()
	}

	// 将预序列化的 messages 反序列化为原生 JSON 类型（[]interface{}），
	// 这样上报给 Langfuse 时才是真正的 JSON 数组，而非 Go string。
	// Langfuse ChatML 适配器会识别 role 字段并分区渲染：
	//   system → 系统提示词（IDE rules / project context）
	//   user   → 用户消息
	//   assistant → 历史回答（可含 tool_calls）
	//   tool   → 工具调用结果（含 tool_call_id）
	var inputData any
	if len(snap.InputMessages) > 0 {
		var msgs any
		if err := json.Unmarshal(snap.InputMessages, &msgs); err == nil {
			inputData = msgs
		}
	}

	outputData := buildOutputData(snap.ResponseContent, snap.ReasoningContent)

	tags := []string{}
	if snap.TokenName != "" {
		tags = append(tags, "token:"+snap.TokenName)
	}
	if snap.UsingGroup != "" {
		tags = append(tags, "group:"+snap.UsingGroup)
	}

	endTime := snap.EndTime
	if endTime.IsZero() {
		endTime = now
	}

	var usageData *UsageData
	if usage != nil {
		usageData = &UsageData{
			Input:  usage.PromptTokens,
			Output: usage.CompletionTokens,
			Total:  usage.TotalTokens,
			Unit:   "TOKENS",
		}
	}

	startTime := snap.StartTime
	var completionStartTime *time.Time
	if !snap.FirstResponseTime.IsZero() {
		t := snap.FirstResponseTime
		completionStartTime = &t
	}

	traceEvent := IngestionEvent{
		ID:        "evt-trace-" + requestId,
		Type:      "trace-create",
		Timestamp: now,
		Body: TraceBody{
			ID:       requestId,
			Name:     snap.OriginModelName,
			UserID:   snap.Username,
			Input:    inputData,
			Output:   outputData,
			Tags:     tags,
			Metadata: buildTraceMetadata(snap),
		},
	}

	generationEvent := IngestionEvent{
		ID:        "evt-gen-" + requestId,
		Type:      "generation-create",
		Timestamp: now,
		Body: GenerationBody{
			ID:                  "gen-" + requestId,
			TraceID:             requestId,
			Name:                snap.RequestURLPath,
			Model:               snap.OriginModelName,
			Input:               inputData,
			Output:              outputData,
			Usage:               usageData,
			StartTime:           &startTime,
			EndTime:             &endTime,
			CompletionStartTime: completionStartTime,
			Metadata:            buildGenerationMetadata(snap, endTime),
		},
	}

	if common.DebugEnabled {
		var usageStr string
		if usageData != nil {
			usageStr = fmt.Sprintf("input=%d, output=%d, total=%d", usageData.Input, usageData.Output, usageData.Total)
		}
		common.SysLog(fmt.Sprintf("Langfuse [enqueue]: requestId=%s, model=%s, user=%s, usage={%s}, ttft=%dms, latency=%dms",
			requestId, snap.OriginModelName, snap.Username, usageStr,
			snap.FirstResponseTime.Sub(snap.StartTime).Milliseconds(),
			endTime.Sub(snap.StartTime).Milliseconds()))
	}

	Enqueue([]IngestionEvent{traceEvent, generationEvent})
}
