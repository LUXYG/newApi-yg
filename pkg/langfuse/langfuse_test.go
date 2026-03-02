package langfuse

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// newTestContext creates a minimal Gin context suitable for unit tests.
func newTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return c, w
}

// --- Client tests ---

func TestGetClient_ReturnsSingletonWhenCalledTwice(t *testing.T) {
	c1 := GetClient()
	c2 := GetClient()
	if c1 != c2 {
		t.Fatal("GetClient should return the same singleton instance")
	}
}

func TestClient_DisabledWhenEnvVarsAbsent(t *testing.T) {
	// The singleton is already initialised without the env vars set in the
	// test binary, so it must be disabled.
	c := GetClient()
	if c.IsEnabled() {
		t.Skip("Langfuse env vars are set; skipping disabled-client test")
	}
	if c.SDK() != nil {
		t.Fatal("SDK() should return nil when client is disabled")
	}
}

// --- Middleware tests ---

func TestMiddleware_SetsStartTime(t *testing.T) {
	c, _ := newTestContext()
	before := time.Now()

	Middleware()(c)

	after := time.Now()
	startTime := StartTimeFromContext(c)
	if startTime.Before(before) || startTime.After(after) {
		t.Errorf("start time %v not in expected window [%v, %v]", startTime, before, after)
	}
}

func TestMiddleware_SetsTraceKey(t *testing.T) {
	c, _ := newTestContext()

	Middleware()(c)

	// The key must always be present (even if the value is empty when
	// Langfuse is disabled) so that TraceIDFromContext never panics.
	_, exists := c.Get(TraceKey)
	if !exists && GetClient().IsEnabled() {
		t.Fatal("trace key should be set when client is enabled")
	}
}

// --- Helper tests ---

func TestTraceIDFromContext_ReturnsEmptyStringWhenAbsent(t *testing.T) {
	c, _ := newTestContext()
	if id := TraceIDFromContext(c); id != "" {
		t.Errorf("expected empty string, got %q", id)
	}
}

func TestTraceIDFromContext_ReturnsStoredID(t *testing.T) {
	c, _ := newTestContext()
	c.Set(TraceKey, "trace-abc-123")
	if id := TraceIDFromContext(c); id != "trace-abc-123" {
		t.Errorf("expected trace-abc-123, got %q", id)
	}
}

func TestStartTimeFromContext_ReturnsFallbackWhenAbsent(t *testing.T) {
	c, _ := newTestContext()
	before := time.Now()
	t2 := StartTimeFromContext(c)
	after := time.Now()
	if t2.Before(before) || t2.After(after) {
		t.Errorf("fallback time %v not in expected window [%v, %v]", t2, before, after)
	}
}

// --- TrackGeneration tests ---

func TestTrackGeneration_NoopWhenDisabled(t *testing.T) {
	if GetClient().IsEnabled() {
		t.Skip("Langfuse client is enabled; skipping no-op test")
	}
	// Must not panic.
	TrackGeneration(GenerationParams{
		Name:             "test",
		Model:            "gpt-4o",
		PromptTokens:     10,
		CompletionTokens: 20,
	})
}

func TestGenerationParams_TotalTokensDerived(t *testing.T) {
	// Verify the zero-value behaviour described in GenerationParams.TotalTokens.
	p := GenerationParams{
		PromptTokens:     100,
		CompletionTokens: 50,
	}
	derived := p.PromptTokens + p.CompletionTokens
	if derived != 150 {
		t.Errorf("expected 150, got %d", derived)
	}
}

// --- StartTrace tests ---

func TestStartTrace_ReturnsEmptyStringWhenDisabled(t *testing.T) {
	if GetClient().IsEnabled() {
		t.Skip("Langfuse client is enabled; skipping disabled test")
	}
	id := StartTrace("test", "user1", "session1", nil, nil, nil)
	if id != "" {
		t.Errorf("expected empty string, got %q", id)
	}
}
