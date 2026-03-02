package langfuse

import (
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ReportGeneration asynchronously sends a trace + generation event pair to Langfuse.
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

	var inputData any
	if info.Request != nil {
		inputData = info.Request
	}

	var outputData any
	if info.ResponseContent != "" {
		outputData = info.ResponseContent
	}

	tags := []string{}
	if tokenName != "" {
		tags = append(tags, "token:"+tokenName)
	}
	if info.UsingGroup != "" {
		tags = append(tags, "group:"+info.UsingGroup)
	}

	metadata := map[string]any{
		"token_id":  info.TokenId,
		"is_stream": info.IsStream,
		"user_id":   info.UserId,
	}
	if info.ChannelMeta != nil {
		metadata["channel_id"] = info.ChannelId
		metadata["channel_type"] = info.ChannelType
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
			Metadata: metadata,
		},
	}

	genMetadata := map[string]any{
		"is_stream":    info.IsStream,
		"relay_mode":   info.RelayMode,
		"relay_format": info.RelayFormat,
	}
	if info.ChannelMeta != nil {
		genMetadata["channel_id"] = info.ChannelId
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
			Metadata:            genMetadata,
		},
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

	infoCopy := &reportSnapshot{
		RequestId:         info.RequestId,
		OriginModelName:   info.OriginModelName,
		RequestURLPath:    info.RequestURLPath,
		IsStream:          info.IsStream,
		RelayMode:         info.RelayMode,
		StartTime:         info.StartTime,
		FirstResponseTime: info.FirstResponseTime,
		UserId:            info.UserId,
		TokenId:           info.TokenId,
		UsingGroup:        info.UsingGroup,
		ResponseContent:   info.ResponseContent,
		Username:          username,
		TokenName:         tokenName,
	}
	if info.ChannelMeta != nil {
		infoCopy.ChannelId = info.ChannelId
		infoCopy.ChannelType = info.ChannelType
	}
	infoCopy.RelayFormat = string(info.RelayFormat)
	if info.Request != nil {
		reqBytes, err := common.Marshal(info.Request)
		if err == nil {
			infoCopy.RequestJSON = string(reqBytes)
		}
	}

	var usageCopy *dto.Usage
	if usage != nil {
		u := *usage
		usageCopy = &u
	}

	go reportFromSnapshot(infoCopy, usageCopy)
}

type reportSnapshot struct {
	RequestId         string
	OriginModelName   string
	RequestURLPath    string
	IsStream          bool
	RelayMode         int
	RelayFormat       string
	StartTime         time.Time
	FirstResponseTime time.Time
	UserId            int
	TokenId           int
	ChannelId         int
	ChannelType       int
	UsingGroup        string
	ResponseContent   string
	Username          string
	TokenName         string
	RequestJSON       string
}

func reportFromSnapshot(snap *reportSnapshot, usage *dto.Usage) {
	now := time.Now()
	requestId := snap.RequestId
	if requestId == "" {
		requestId = uuid.New().String()
	}

	var inputData any
	if snap.RequestJSON != "" {
		inputData = snap.RequestJSON
	}

	var outputData any
	if snap.ResponseContent != "" {
		outputData = snap.ResponseContent
	}

	tags := []string{}
	if snap.TokenName != "" {
		tags = append(tags, "token:"+snap.TokenName)
	}
	if snap.UsingGroup != "" {
		tags = append(tags, "group:"+snap.UsingGroup)
	}

	metadata := map[string]any{
		"channel_id":   snap.ChannelId,
		"channel_type": snap.ChannelType,
		"token_id":     snap.TokenId,
		"is_stream":    snap.IsStream,
		"user_id":      snap.UserId,
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
			Metadata: metadata,
		},
	}

	genMetadata := map[string]any{
		"channel_id":   snap.ChannelId,
		"is_stream":    snap.IsStream,
		"relay_mode":   snap.RelayMode,
		"relay_format": snap.RelayFormat,
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
			EndTime:             &now,
			CompletionStartTime: completionStartTime,
			Metadata:            genMetadata,
		},
	}

	Enqueue([]IngestionEvent{traceEvent, generationEvent})
}
