package langfuse

import (
	"context"
	"time"

	lfmodel "github.com/henomis/langfuse-go/model"
)

// GenerationParams carries the information about a single LLM call that will
// be forwarded to Langfuse as a "generation" observation inside a trace.
type GenerationParams struct {
	// TraceID ties this generation to an existing trace created by StartTrace.
	// When empty a new trace is created automatically.
	TraceID string

	// Name is a human-readable label for the generation (e.g. "chat-completion").
	Name string

	// Model is the canonical model identifier as returned by the upstream
	// provider (e.g. "gpt-4o", "claude-3-5-sonnet-20241022").
	Model string

	// Input is the list of messages sent to the model.  Pass the raw
	// []map[string]any slice from the OpenAI-compatible request.
	Input any

	// Output is the completion content returned by the model.
	Output any

	// PromptTokens is the number of tokens in the prompt.
	PromptTokens int

	// CompletionTokens is the number of tokens in the completion.
	CompletionTokens int

	// TotalTokens is the combined token count.  When zero it is derived from
	// PromptTokens + CompletionTokens.
	TotalTokens int

	// StartTime is when the upstream request was dispatched.
	StartTime time.Time

	// EndTime is when the upstream response was fully received.
	EndTime time.Time

	// UserID is an optional identifier for the end-user that triggered the
	// request.  Used to populate the trace's userId field.
	UserID string

	// SessionID groups multiple traces that belong to the same conversation.
	SessionID string

	// Metadata holds arbitrary key/value pairs that are forwarded verbatim to
	// Langfuse (e.g. channel_id, token_name, relay_mode).
	Metadata map[string]any

	// Tags are free-form labels attached to the trace (e.g. ["streaming"]).
	Tags []string

	// ModelParameters holds sampling parameters such as temperature, max_tokens.
	ModelParameters map[string]any
}

// TrackGeneration records a completed LLM generation in Langfuse.
// It creates a new trace and a generation observation within that trace.
// The call is a no-op when the Langfuse client is disabled.
func TrackGeneration(params GenerationParams) {
	client := GetClient()
	if !client.IsEnabled() {
		return
	}

	lf := client.SDK()

	now := time.Now()
	startTime := params.StartTime
	if startTime.IsZero() {
		startTime = now
	}
	endTime := params.EndTime
	if endTime.IsZero() {
		endTime = now
	}

	totalTokens := params.TotalTokens
	if totalTokens == 0 {
		totalTokens = params.PromptTokens + params.CompletionTokens
	}

	name := params.Name
	if name == "" {
		name = "llm-generation"
	}

	traceID := params.TraceID
	if traceID == "" {
		trace := &lfmodel.Trace{
			Name:      name,
			UserID:    params.UserID,
			SessionID: params.SessionID,
			Input:     params.Input,
			Output:    params.Output,
			Metadata:  params.Metadata,
			Tags:      params.Tags,
		}
		t, err := lf.Trace(trace)
		if err != nil {
			return
		}
		traceID = t.ID
	}

	generation := &lfmodel.Generation{
		TraceID:         traceID,
		Name:            name,
		Model:           params.Model,
		Input:           params.Input,
		Output:          params.Output,
		StartTime:       &startTime,
		EndTime:         &endTime,
		ModelParameters: params.ModelParameters,
		Metadata:        params.Metadata,
		Usage: lfmodel.Usage{
			PromptTokens:     params.PromptTokens,
			CompletionTokens: params.CompletionTokens,
			TotalTokens:      totalTokens,
			Input:            params.PromptTokens,
			Output:           params.CompletionTokens,
			Total:            totalTokens,
			Unit:             lfmodel.ModelUsageUnitTokens,
		},
	}

	g, err := lf.Generation(generation, nil)
	if err != nil {
		return
	}

	// Finalise the generation: Langfuse's GenerationEnd sends an "update"
	// event that marks the observation as complete with its end time.
	// The ID and TraceID are required by the SDK for the update event.
	_, _ = lf.GenerationEnd(&lfmodel.Generation{
		ID:      g.ID,
		TraceID: traceID,
	})

	lf.Flush(context.Background())
}

// StartTrace creates a Langfuse trace and returns its ID.
// The returned traceID should be stored in the Gin context (via TraceKey) and
// passed to TrackGeneration once the LLM response is available.
// Returns an empty string when the client is disabled.
func StartTrace(name, userID, sessionID string, input any, metadata map[string]any, tags []string) string {
	client := GetClient()
	if !client.IsEnabled() {
		return ""
	}

	trace := &lfmodel.Trace{
		Name:      name,
		UserID:    userID,
		SessionID: sessionID,
		Input:     input,
		Metadata:  metadata,
		Tags:      tags,
	}
	t, err := client.SDK().Trace(trace)
	if err != nil {
		return ""
	}
	return t.ID
}
