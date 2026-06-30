// Package model содержит основные доменные типы, используемые во всём сервисе сокращения ссылок.
package model

// ShortURL — основная модель хранилища, связывающая сгенерированный короткий идентификатор
// с оригинальным длинным URL и привязанная к конкретному пользователю.
type ShortURL struct {
	// UUID — суррогатный ключ с автоинкрементом, присваиваемый слоем хранилища.
	UUID int `json:"uuid" db:"uuid"`
	// ShortURL — сгенерированный короткий идентификатор (например, "aB3xYz12"), не полный URL.
	ShortURL string `json:"short_url" db:"short_url"`
	// OriginalURL — нормализованный длинный URL, на который ссылается короткий идентификатор.
	OriginalURL string `json:"original_url" db:"original_url"`
	// UserID — идентификатор владельца записи; извлекается из подписанной auth-куки.
	UserID string `json:"user_id" db:"user_id"`
	// DeletedFlag помечает запись как мягко удалённую; переходы по таким ссылкам
	// возвращают HTTP 410 Gone.
	DeletedFlag bool `json:"is_deleted" db:"is_deleted"`
}

// BatchRequestItem — один элемент пакетного запроса на сокращение.
// CorrelationID — произвольная строка вызывающей стороны, которая возвращается
// в ответе, чтобы сопоставить входные данные с результатами.
type BatchRequestItem struct {
	CorrelationID string `json:"correlation_id"`
	OriginalURL   string `json:"original_url"`
}

// BatchResponseItem — один элемент пакетного ответа на сокращение.
// Содержит CorrelationID от вызывающей стороны и итоговый короткий URL.
type BatchResponseItem struct {
	CorrelationID string `json:"correlation_id"`
	ShortURL      string `json:"short_url"`
}

// UserURLResponse — одна запись в списке, возвращаемом GET /api/user/urls.
// Оба поля содержат абсолютные URL.
type UserURLResponse struct {
	ShortURL    string `json:"short_url"`
	OriginalURL string `json:"original_url"`
}
