package audit

import (
	"context"
	"errors"
)

// Auditor — интерфейс наблюдателя аудита.
type Auditor interface {
	Emit(ctx context.Context, e Event) error
}

// NoopAuditor используется, когда приемники не настроены.
type NoopAuditor struct{}

func (NoopAuditor) Emit(_ context.Context, _ Event) error { return nil }

// MultiAuditor рассылает событие всем зарегистрированным наблюдателям.
type MultiAuditor struct {
	sinks []Auditor
}

func NewMultiAuditor(sinks ...Auditor) *MultiAuditor {
	return &MultiAuditor{sinks: sinks}
}

func (m *MultiAuditor) Emit(ctx context.Context, e Event) error {
	var errs []error
	for _, s := range m.sinks {
		if err := s.Emit(ctx, e); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
