package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	retryablehttp "github.com/hashicorp/go-retryablehttp"
)

const (
	auditHTTPTimeout  = 5 * time.Second
	auditRetryMax     = 3
	auditRetryWaitMin = 200 * time.Millisecond
	auditRetryWaitMax = 2 * time.Second
)

// RemoteAuditor отправляет события аудита на удаленный HTTP-сервер методом POST.
// При временных сбоях (5xx, сетевые ошибки) автоматически повторяет запрос до auditRetryMax раз.
type RemoteAuditor struct {
	url    string
	client *http.Client
}

func NewRemoteAuditor(url string) *RemoteAuditor {
	rc := retryablehttp.NewClient()
	rc.RetryMax = auditRetryMax
	rc.RetryWaitMin = auditRetryWaitMin
	rc.RetryWaitMax = auditRetryWaitMax
	rc.HTTPClient.Timeout = auditHTTPTimeout
	rc.Logger = nil // подавляем стандартный вывод go-retryablehttp в log.Printf
	rc.CheckRetry = auditRetryPolicy
	return &RemoteAuditor{
		url:    url,
		client: rc.StandardClient(),
	}
}

// auditRetryPolicy повторяет запрос при сетевых ошибках и ответах 5xx.
// Ответы 4xx считаются постоянными ошибками — ретрай не имеет смысла.
func auditRetryPolicy(ctx context.Context, resp *http.Response, err error) (bool, error) {
	retry, err := retryablehttp.DefaultRetryPolicy(ctx, resp, err)
	if !retry || err != nil {
		return retry, err
	}
	// DefaultRetryPolicy уже обрабатывает 429 и 5xx; дополнительно дренируем тело,
	// чтобы соединение вернулось в пул до следующей попытки.
	if resp != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
	}
	return true, nil
}

func (ra *RemoteAuditor) Close() error { return nil }

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
