package audit

// Event — событие аудита, формируется после успешной обработки запроса.
type Event struct {
	Ts     int64  `json:"ts"`
	Action string `json:"action"`
	UserID string `json:"user_id,omitempty"`
	URL    string `json:"url"`
}
