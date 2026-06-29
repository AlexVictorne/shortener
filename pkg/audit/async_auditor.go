package audit

import (
	"context"
	"sync"

	"github.com/rs/zerolog"
)

const defaultAsyncBufSize = 256

// AsyncAuditor оборачивает любой Auditor и делает Emit неблокирующим:
// событие кладется в буферизованный канал и обрабатывается единственной
// горутиной-воркером. Если канал заполнен — событие отбрасывается с предупреждением.
// Close дренирует канал и ждет завершения воркера перед закрытием внутреннего Auditor.
type AsyncAuditor struct {
	inner  Auditor
	ch     chan Event
	wg     sync.WaitGroup
	logger zerolog.Logger
}

// NewAsyncAuditor создаёт AsyncAuditor с буфером bufSize событий и запускает воркер.
func NewAsyncAuditor(inner Auditor, bufSize int, logger zerolog.Logger) *AsyncAuditor {
	if bufSize <= 0 {
		bufSize = defaultAsyncBufSize
	}
	a := &AsyncAuditor{
		inner:  inner,
		ch:     make(chan Event, bufSize),
		logger: logger,
	}
	a.wg.Add(1)
	go a.run()
	return a
}

func (a *AsyncAuditor) run() {
	defer a.wg.Done()
	for e := range a.ch {
		if err := a.inner.Emit(context.Background(), e); err != nil {
			a.logger.Warn().Err(err).Msg("audit emit failed")
		}
	}
}

// Emit помещает событие в канал и немедленно возвращает управление вызывающей стороне.
// Если канал переполнен — событие отбрасывается, ошибка не возвращается,
// чтобы не блокировать hot path.
func (a *AsyncAuditor) Emit(_ context.Context, e Event) error {
	select {
	case a.ch <- e:
	default:
		a.logger.Warn().Str("action", e.Action).Msg("audit channel full, event dropped")
	}
	return nil
}

// Close закрывает канал, ждет обработки всех оставшихся событий и закрывает внутренний Auditor.
func (a *AsyncAuditor) Close() error {
	close(a.ch)
	a.wg.Wait()
	return a.inner.Close()
}
