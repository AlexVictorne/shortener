package generator

type IDGenerator interface {
	GenerateID() (string, error)
	Validate(id string) bool
}
