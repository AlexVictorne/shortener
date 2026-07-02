package options

//go:generate go run github.com/kazhuravlev/options-gen/cmd/options-gen -filename=options.go -from-struct=HandlerOptions -out-filename=options_gen.go -pkg=options

import (
	"context"

	"shortener/pkg/audit"
)

// Pinger описывает зависимость от бэкенда хранилища, способного проверить свое состояние.
type Pinger interface {
	Ping(ctx context.Context) error
}

type HandlerOptions struct {
	AuthSecret string
	Pinger     Pinger
	Auditor    audit.Auditor
}
