package audit

// Event — событие аудита, формируется после успешной обработки запроса.
// Сериализуется в JSON и записывается приёмником (FileAuditor или RemoteAuditor).
type Event struct {
	// TS — Unix-время события в секундах.
	TS int64 `json:"ts"`
	// Action — тип действия: "shorten" (создание) или "follow" (переход по ссылке).
	Action string `json:"action"`
	// UserID — идентификатор пользователя из auth-куки; может быть пустым для анонимных запросов.
	UserID string `json:"user_id,omitempty"`
	// URL — оригинальный URL, к которому относится событие.
	URL string `json:"url"`
}
