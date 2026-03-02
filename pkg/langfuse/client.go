package langfuse

import (
	"context"
	"os"
	"sync"

	lfgo "github.com/henomis/langfuse-go"
)

// Client wraps the Langfuse SDK client with a lazy-initialised singleton.
// When the required environment variables are absent the client is disabled
// and all tracking calls become no-ops, so the gateway continues to operate
// without a Langfuse deployment.
type Client struct {
	lf      *lfgo.Langfuse
	enabled bool
}

var (
	globalClient *Client
	once         sync.Once
)

// GetClient returns the process-wide singleton Langfuse client.
// The client is enabled only when all three environment variables are set:
// LANGFUSE_HOST, LANGFUSE_PUBLIC_KEY, LANGFUSE_SECRET_KEY.
func GetClient() *Client {
	once.Do(func() {
		host := os.Getenv("LANGFUSE_HOST")
		publicKey := os.Getenv("LANGFUSE_PUBLIC_KEY")
		secretKey := os.Getenv("LANGFUSE_SECRET_KEY")

		enabled := host != "" && publicKey != "" && secretKey != ""
		globalClient = &Client{enabled: enabled}
		if enabled {
			globalClient.lf = lfgo.New(context.Background())
		}
	})
	return globalClient
}

// IsEnabled returns true when the Langfuse client is properly configured.
func (c *Client) IsEnabled() bool {
	return c.enabled
}

// SDK returns the underlying Langfuse SDK instance.
// Returns nil when the client is disabled.
func (c *Client) SDK() *lfgo.Langfuse {
	return c.lf
}
