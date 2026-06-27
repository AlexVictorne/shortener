// Package generator предоставляет интерфейс IDGenerator и его реализацию на основе crypto/rand.
// Сгенерированные идентификаторы кодируются в URL-safe base64 и проверяются регулярным выражением.
package generator

// IDGenerator — интерфейс генератора коротких идентификаторов.
type IDGenerator interface {
	// GenerateID генерирует новый уникальный идентификатор, пригодный для использования в URL.
	GenerateID() (string, error)
	// Validate проверяет, что id соответствует ожидаемому формату (URL-safe base64).
	Validate(id string) bool
}
