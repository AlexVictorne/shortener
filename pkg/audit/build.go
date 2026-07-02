package audit

import (
	"fmt"

	"github.com/rs/zerolog"
)

// Build создает Auditor на основе переданных параметров конфигурации.
// Если ни один параметр не задан — возвращает NoopAuditor.
// Возвращает ошибку, если файловый приёмник настроен, но открыть файл не удалось.
func Build(auditFile, auditURL string, logger zerolog.Logger) (Auditor, error) {
	var sinks []Auditor

	if auditFile != "" {
		fa, err := NewFileAuditor(auditFile)
		if err != nil {
			return nil, fmt.Errorf("audit: не удалось инициализировать файловый приёмник %q: %w", auditFile, err)
		}
		sinks = append(sinks, fa)
	}

	if auditURL != "" {
		sinks = append(sinks, NewRemoteAuditor(auditURL))
	}

	switch len(sinks) {
	case 0:
		return NoopAuditor{}, nil
	case 1:
		return NewAsyncAuditor(sinks[0], defaultAsyncBufSize, logger), nil
	default:
		return NewAsyncAuditor(NewMultiAuditor(sinks...), defaultAsyncBufSize, logger), nil
	}
}
