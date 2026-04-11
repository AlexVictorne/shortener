package options

//go:generate go run github.com/kazhuravlev/options-gen/cmd/options-gen -filename=options.go -from-struct=HandlerOptions -out-filename=options_gen.go -pkg=options

type HandlerOptions struct {
	AuthSecret string
	Pinger     interface{}
}
