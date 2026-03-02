package langfuse

import "time"

// IngestionRequest is the top-level batch payload sent to POST /api/public/ingestion.
type IngestionRequest struct {
	Batch []IngestionEvent `json:"batch"`
}

// IngestionEvent is a single event in the batch, discriminated by Type.
type IngestionEvent struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Body      any       `json:"body"`
}

// TraceBody is the body for a "trace-create" event.
type TraceBody struct {
	ID       string            `json:"id"`
	Name     string            `json:"name,omitempty"`
	UserID   string            `json:"userId,omitempty"`
	Input    any               `json:"input,omitempty"`
	Output   any               `json:"output,omitempty"`
	Tags     []string          `json:"tags,omitempty"`
	Metadata map[string]any    `json:"metadata,omitempty"`
}

// GenerationBody is the body for a "generation-create" event.
type GenerationBody struct {
	ID                  string         `json:"id"`
	TraceID             string         `json:"traceId,omitempty"`
	Name                string         `json:"name,omitempty"`
	Model               string         `json:"model,omitempty"`
	Input               any            `json:"input,omitempty"`
	Output              any            `json:"output,omitempty"`
	Usage               *UsageData     `json:"usage,omitempty"`
	StartTime           *time.Time     `json:"startTime,omitempty"`
	EndTime             *time.Time     `json:"endTime,omitempty"`
	CompletionStartTime *time.Time     `json:"completionStartTime,omitempty"`
	Metadata            map[string]any `json:"metadata,omitempty"`
}

// UsageData holds token usage info for a generation.
type UsageData struct {
	Input  int    `json:"input"`
	Output int    `json:"output"`
	Total  int    `json:"total"`
	Unit   string `json:"unit"`
}

// IngestionResponse is the response from the ingestion endpoint (207 Multi-Status).
type IngestionResponse struct {
	Successes []IngestionResult `json:"successes"`
	Errors    []IngestionResult `json:"errors"`
}

// IngestionResult represents a single event result.
type IngestionResult struct {
	ID      string `json:"id"`
	Status  int    `json:"status"`
	Message string `json:"message,omitempty"`
}
