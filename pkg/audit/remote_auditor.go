package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// RemoteAuditor отправляет события аудита на удалённый HTTP-сервер методом POST.
type RemoteAuditor struct {
	url    string
	client *http.Client
}

func NewRemoteAuditor(url string) *RemoteAuditor {
	return &RemoteAuditor{
		url:    url,
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

func (ra *RemoteAuditor) Emit(ctx context.Context, e Event) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ra.url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := ra.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("audit remote: unexpected status %d", resp.StatusCode)
	}
	return nil
}
