package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// postJSON sends a POST with retries on 429 and 5xx.
func postJSON(ctx context.Context, client *http.Client, url string, headers map[string]string, body []byte, dest interface{}) error {
	var lastErr error
	backoff := 300 * time.Millisecond
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		rb, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("http %d: %s", resp.StatusCode, string(bytes.TrimSpace(rb)))
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("http %d: %s", resp.StatusCode, string(bytes.TrimSpace(rb)))
		}
		if dest != nil {
			if err := json.Unmarshal(rb, dest); err != nil {
				return fmt.Errorf("decode: %w", err)
			}
		}
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("exhausted retries")
}
