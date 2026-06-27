package options

//go:generate go run github.com/kazhuravlev/options-gen/cmd/options-gen -filename=options.go -from-struct=HandlerOptions -out-filename=options_gen.go -pkg=options

// HandlerOptions содержит параметры для настройки Handler.
type HandlerOptions struct {
	AuthSecret string
	Pinger     interface{}
	Auditor    interface{}
	// TrustedSubnet — строковый CIDR доверенной подсети для эндпоинта /api/internal/stats.
	// Пустое значение запрещает доступ к эндпоинту для любых запросов.
	TrustedSubnet string
}
