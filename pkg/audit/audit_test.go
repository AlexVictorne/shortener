package audit_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"shortener/pkg/audit"
)

func nopLogger() zerolog.Logger { return zerolog.Nop() }

// --- NoopAuditor ---

func TestNoopAuditor_Emit(t *testing.T) {
	var a audit.Auditor = audit.NoopAuditor{}
	err := a.Emit(context.Background(), audit.Event{TS: 1, Action: "shorten", URL: "https://ya.ru"})
	assert.NoError(t, err)
}

// --- FileAuditor ---

func TestFileAuditor_Emit_WritesJSONLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")

	fa, err := audit.NewFileAuditor(path)
	require.NoError(t, err)

	e := audit.Event{TS: 1700000000, Action: "shorten", UserID: "u1", URL: "https://ya.ru"}
	require.NoError(t, fa.Emit(context.Background(), e))

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var got audit.Event
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, e, got)
}

func TestFileAuditor_Emit_AppendsMultipleLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")

	fa, err := audit.NewFileAuditor(path)
	require.NoError(t, err)

	events := []audit.Event{
		{TS: 1, Action: "shorten", URL: "https://ya.ru"},
		{TS: 2, Action: "follow", URL: "https://yandex.ru"},
		{TS: 3, Action: "shorten", UserID: "u1", URL: "https://google.com"},
	}
	for _, e := range events {
		require.NoError(t, fa.Emit(context.Background(), e))
	}

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	lines := splitLines(data)
	require.Len(t, lines, len(events))

	for i, line := range lines {
		var got audit.Event
		require.NoError(t, json.Unmarshal([]byte(line), &got))
		assert.Equal(t, events[i], got)
	}
}

func TestFileAuditor_Emit_OmitsEmptyUserID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")

	fa, err := audit.NewFileAuditor(path)
	require.NoError(t, err)

	require.NoError(t, fa.Emit(context.Background(), audit.Event{TS: 1, Action: "shorten", URL: "https://ya.ru"}))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "user_id")
}

func TestNewFileAuditor_InvalidPath(t *testing.T) {
	_, err := audit.NewFileAuditor("/no/such/dir/audit.log")
	assert.Error(t, err)
}

// --- RemoteAuditor ---

func TestRemoteAuditor_Emit_SendsPostJSON(t *testing.T) {
	var received []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		received, _ = readAll(r)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ra := audit.NewRemoteAuditor(srv.URL)
	e := audit.Event{TS: 999, Action: "follow", UserID: "u2", URL: "https://example.com"}
	require.NoError(t, ra.Emit(context.Background(), e))

	var got audit.Event
	require.NoError(t, json.Unmarshal(received, &got))
	assert.Equal(t, e, got)
}

func TestRemoteAuditor_Emit_Non2xxReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ra := audit.NewRemoteAuditor(srv.URL)
	err := ra.Emit(context.Background(), audit.Event{TS: 1, Action: "shorten", URL: "https://ya.ru"})
	assert.Error(t, err)
}

func TestRemoteAuditor_Emit_ConnectionRefusedReturnsError(t *testing.T) {
	ra := audit.NewRemoteAuditor("http://127.0.0.1:1") // заведомо недоступный порт
	err := ra.Emit(context.Background(), audit.Event{TS: 1, Action: "shorten", URL: "https://ya.ru"})
	assert.Error(t, err)
}

// --- MultiAuditor ---

func TestMultiAuditor_Emit_CallsAllSinks(t *testing.T) {
	calls := make([]string, 0, 2)
	a1 := &spyAuditor{label: "a1", calls: &calls}
	a2 := &spyAuditor{label: "a2", calls: &calls}

	m := audit.NewMultiAuditor(a1, a2)
	require.NoError(t, m.Emit(context.Background(), audit.Event{TS: 1, Action: "shorten", URL: "https://ya.ru"}))

	assert.ElementsMatch(t, []string{"a1", "a2"}, calls)
}

func TestMultiAuditor_Emit_CollectsAllErrors(t *testing.T) {
	err1 := errors.New("sink1 error")
	err2 := errors.New("sink2 error")
	a1 := &errorAuditor{err: err1}
	a2 := &errorAuditor{err: err2}

	m := audit.NewMultiAuditor(a1, a2)
	err := m.Emit(context.Background(), audit.Event{TS: 1, Action: "shorten", URL: "https://ya.ru"})
	require.Error(t, err)
	assert.ErrorIs(t, err, err1)
	assert.ErrorIs(t, err, err2)
}

func TestMultiAuditor_Emit_ContinuesOnPartialError(t *testing.T) {
	calls := make([]string, 0, 1)
	a1 := &errorAuditor{err: errors.New("fail")}
	a2 := &spyAuditor{label: "a2", calls: &calls}

	m := audit.NewMultiAuditor(a1, a2)
	err := m.Emit(context.Background(), audit.Event{TS: 1, Action: "shorten", URL: "https://ya.ru"})
	assert.Error(t, err)
	assert.Contains(t, calls, "a2") // второй наблюдатель все равно вызван
}

// --- Build ---

func TestBuild_NeitherParam_ReturnsNoop(t *testing.T) {
	a, err := audit.Build("", "", nopLogger())
	require.NoError(t, err)
	assert.IsType(t, audit.NoopAuditor{}, a)
}

func TestBuild_OnlyFile_ReturnsFileAuditor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	a, err := audit.Build(path, "", nopLogger())
	require.NoError(t, err)
	require.NoError(t, a.Emit(context.Background(), audit.Event{TS: 1, Action: "shorten", URL: "https://ya.ru"}))
	require.NoError(t, a.Close())
	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.NotEmpty(t, data)
}

func TestBuild_OnlyURL_ReturnsRemoteAuditor(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	a, err := audit.Build("", srv.URL, nopLogger())
	require.NoError(t, err)
	require.NoError(t, a.Emit(context.Background(), audit.Event{TS: 1, Action: "follow", URL: "https://ya.ru"}))
	require.NoError(t, a.Close()) // дренируем канал до закрытия сервера
	srv.Close()

	assert.True(t, called)
}

func TestBuild_BothParams_ReturnsMultiAuditor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")

	remoteCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		remoteCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	a, err := audit.Build(path, srv.URL, nopLogger())
	require.NoError(t, err)
	require.NoError(t, a.Emit(context.Background(), audit.Event{TS: 1, Action: "shorten", URL: "https://ya.ru"}))
	require.NoError(t, a.Close()) // дренируем канал до закрытия сервера
	srv.Close()

	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.NotEmpty(t, data)
	assert.True(t, remoteCalled)
}

func TestBuild_InvalidFilePath_ReturnsError(t *testing.T) {
	_, err := audit.Build("/no/such/dir/audit.log", "", nopLogger())
	assert.Error(t, err)
}

// --- helpers ---

type spyAuditor struct {
	label string
	calls *[]string
}

func (s *spyAuditor) Close() error { return nil }

func (s *spyAuditor) Emit(_ context.Context, _ audit.Event) error {
	*s.calls = append(*s.calls, s.label)
	return nil
}

type errorAuditor struct{ err error }

func (e *errorAuditor) Close() error                                { return nil }
func (e *errorAuditor) Emit(_ context.Context, _ audit.Event) error { return e.err }

func readAll(r *http.Request) ([]byte, error) {
	buf := make([]byte, 0, 512)
	tmp := make([]byte, 512)
	for {
		n, err := r.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	return buf, nil
}

// --- AsyncAuditor ---

func TestAsyncAuditor_Emit_DeliveredToInner(t *testing.T) {
	calls := make([]string, 0, 1)
	inner := &spyAuditor{label: "inner", calls: &calls}

	a := audit.NewAsyncAuditor(inner, 8, nopLogger())
	err := a.Emit(context.Background(), audit.Event{TS: 1, Action: "shorten", URL: "https://ya.ru"})
	require.NoError(t, err)
	require.NoError(t, a.Close())

	assert.Contains(t, calls, "inner")
}

func TestAsyncAuditor_Emit_NonBlocking(t *testing.T) {
	// slow — блокирует воркер на каждом событии
	slow := &slowAuditor{delay: 50 * time.Millisecond}
	a := audit.NewAsyncAuditor(slow, 8, nopLogger())

	start := time.Now()
	for i := 0; i < 4; i++ {
		require.NoError(t, a.Emit(context.Background(), audit.Event{TS: int64(i), Action: "follow", URL: "https://ya.ru"}))
	}
	elapsed := time.Since(start)
	_ = a.Close()

	// Все 4 вызова Emit должны вернуться немедленно, не ждя воркер
	assert.Less(t, elapsed, 20*time.Millisecond, "Emit должен быть неблокирующим")
}

func TestAsyncAuditor_Close_DrainsChannel(t *testing.T) {
	var mu sync.Mutex
	received := make([]int64, 0, 10)
	inner := &collectAuditor{mu: &mu, received: &received}

	a := audit.NewAsyncAuditor(inner, 64, nopLogger())
	const n = 10
	for i := 0; i < n; i++ {
		require.NoError(t, a.Emit(context.Background(), audit.Event{TS: int64(i), Action: "shorten", URL: "https://ya.ru"}))
	}
	require.NoError(t, a.Close())

	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, received, n, "Close должен дождаться обработки всех событий из канала")
}

func TestAsyncAuditor_FullChannel_DropsWithoutBlock(t *testing.T) {
	slow := &slowAuditor{delay: 100 * time.Millisecond}
	const bufSize = 2
	a := audit.NewAsyncAuditor(slow, bufSize, nopLogger())

	// Заполняем канал + 1 лишний — должен быть отброшен без блокировки
	start := time.Now()
	for i := 0; i < bufSize+2; i++ {
		_ = a.Emit(context.Background(), audit.Event{TS: int64(i), Action: "follow", URL: "https://ya.ru"})
	}
	elapsed := time.Since(start)
	_ = a.Close()

	assert.Less(t, elapsed, 50*time.Millisecond, "Emit при переполненном канале не должен блокировать")
}

func TestBuild_NonNoop_ReturnsAsyncAuditor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	a, err := audit.Build(path, "", nopLogger())
	require.NoError(t, err)
	assert.IsType(t, &audit.AsyncAuditor{}, a)
	_ = a.Close()
}

// --- дополнительные helpers ---

type slowAuditor struct{ delay time.Duration }

func (s *slowAuditor) Close() error { return nil }
func (s *slowAuditor) Emit(_ context.Context, _ audit.Event) error {
	time.Sleep(s.delay)
	return nil
}

type collectAuditor struct {
	mu       *sync.Mutex
	received *[]int64
}

func (c *collectAuditor) Close() error { return nil }
func (c *collectAuditor) Emit(_ context.Context, e audit.Event) error {
	c.mu.Lock()
	*c.received = append(*c.received, e.TS)
	c.mu.Unlock()
	return nil
}

// splitLines разбивает байты на непустые строки.
func splitLines(data []byte) []string {
	var lines []string
	start := 0
	for i, b := range data {
		if b == '\n' {
			if i > start {
				lines = append(lines, string(data[start:i]))
			}
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, string(data[start:]))
	}
	return lines
}
