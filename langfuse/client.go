package langfuse

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

var (
	globalClient *Client
	once         sync.Once
	flushOnce    sync.Once
)

// Client is a singleton Langfuse ingestion client with async batched sending.
type Client struct {
	publicKey  string
	secretKey  string
	baseURL    string
	httpClient *http.Client

	eventCh    chan []IngestionEvent
	batchSize  int
	flushInterval time.Duration

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// Init initialises the global Langfuse client from environment variables.
// If LANGFUSE_PUBLIC_KEY or LANGFUSE_SECRET_KEY is empty the client stays nil
// and all Report* calls become no-ops.
func Init() {
	once.Do(func() {
		publicKey := os.Getenv("LANGFUSE_PUBLIC_KEY")
		secretKey := os.Getenv("LANGFUSE_SECRET_KEY")
		baseURL := os.Getenv("LANGFUSE_BASE_URL")

		if publicKey == "" || secretKey == "" {
			common.SysLog("Langfuse: disabled (LANGFUSE_PUBLIC_KEY or LANGFUSE_SECRET_KEY not set)")
			return
		}

		if enabled := os.Getenv("LANGFUSE_ENABLED"); enabled == "false" {
			common.SysLog("Langfuse: disabled by LANGFUSE_ENABLED=false")
			return
		}

		if baseURL == "" {
			baseURL = "https://cloud.langfuse.com"
		}

		batchSize := 50
		if v := os.Getenv("LANGFUSE_BATCH_SIZE"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				batchSize = n
			}
		}

		flushInterval := 5 * time.Second
		if v := os.Getenv("LANGFUSE_FLUSH_INTERVAL"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				flushInterval = time.Duration(n) * time.Second
			}
		}

		c := &Client{
			publicKey:     publicKey,
			secretKey:     secretKey,
			baseURL:       baseURL,
			httpClient:    &http.Client{Timeout: 10 * time.Second},
			eventCh:       make(chan []IngestionEvent, 1024),
			batchSize:     batchSize,
			flushInterval: flushInterval,
			stopCh:        make(chan struct{}),
		}

		c.wg.Add(1)
		go c.worker()

		globalClient = c
		common.SysLog(fmt.Sprintf("Langfuse: enabled, base_url=%s, batch_size=%d, flush_interval=%v", baseURL, batchSize, flushInterval))
	})
}

// Flush drains pending events and shuts down the background worker.
// Safe to call multiple times and when the client is nil (disabled).
func Flush() {
	if globalClient == nil {
		return
	}
	flushOnce.Do(func() {
		close(globalClient.stopCh)
		globalClient.wg.Wait()
		common.SysLog("Langfuse: flushed and stopped")
	})
}

// Enqueue adds a set of events to the send queue.
// Non-blocking: if the channel is full the events are dropped with a warning.
func Enqueue(events []IngestionEvent) {
	if globalClient == nil || len(events) == 0 {
		return
	}
	if common.DebugEnabled {
		for _, e := range events {
			common.SysLog(fmt.Sprintf("Langfuse [enqueue]: type=%s, id=%s", e.Type, e.ID))
		}
	}
	select {
	case globalClient.eventCh <- events:
	default:
		common.SysError("Langfuse: event queue full, dropping events")
	}
}

// worker runs in a background goroutine, batching and sending events.
func (c *Client) worker() {
	defer c.wg.Done()

	buf := make([]IngestionEvent, 0, c.batchSize)
	ticker := time.NewTicker(c.flushInterval)
	defer ticker.Stop()

	flush := func() {
		if len(buf) == 0 {
			return
		}
		toSend := make([]IngestionEvent, len(buf))
		copy(toSend, buf)
		buf = buf[:0]
		c.send(toSend)
	}

	for {
		select {
		case events := <-c.eventCh:
			buf = append(buf, events...)
			if len(buf) >= c.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-c.stopCh:
			// drain remaining events from channel
			for {
				select {
				case events := <-c.eventCh:
					buf = append(buf, events...)
				default:
					flush()
					return
				}
			}
		}
	}
}

// send posts a batch of events to the Langfuse ingestion API.
func (c *Client) send(events []IngestionEvent) {
	req := IngestionRequest{Batch: events}
	body, err := common.Marshal(req)
	if err != nil {
		common.SysError(fmt.Sprintf("Langfuse: marshal error: %v", err))
		return
	}

	httpReq, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/public/ingestion", bytes.NewReader(body))
	if err != nil {
		common.SysError(fmt.Sprintf("Langfuse: new request error: %v", err))
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.SetBasicAuth(c.publicKey, c.secretKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		common.SysError(fmt.Sprintf("Langfuse: send error: %v", err))
		return
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusMultiStatus {
		common.SysError(fmt.Sprintf("Langfuse: unexpected status %d for %d events", resp.StatusCode, len(events)))
	} else if common.DebugEnabled {
		common.SysLog(fmt.Sprintf("Langfuse [send]: status=%d, events=%d, bodySize=%d bytes", resp.StatusCode, len(events), len(body)))
	}
}
