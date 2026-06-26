package audit

import "github.com/rs/zerolog/log"

// Build создаёт Auditor на основе переданных параметров конфигурации.
// Если ни один параметр не задан — возвращает NoopAuditor.
func Build(auditFile, auditURL string) Auditor {
	var sinks []Auditor

	if auditFile != "" {
		fa, err := NewFileAuditor(auditFile)
		if err != nil {
			log.Warn().Err(err).Str("path", auditFile).Msg("audit: не удалось инициализировать файловый приёмник")
		} else {
			sinks = append(sinks, fa)
		}
	}

	if auditURL != "" {
		sinks = append(sinks, NewRemoteAuditor(auditURL))
	}

	switch len(sinks) {
	case 0:
		return NoopAuditor{}
	case 1:
		return sinks[0]
	default:
		return NewMultiAuditor(sinks...)
	}
}
