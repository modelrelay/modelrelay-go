package wrapperv1

// APIError allows adapters to return a structured error payload.
type APIError struct {
	Status       int
	Code         string
	Message      string
	RetryAfterMS *int64
}

func (e *APIError) Error() string {
	return e.Message
}
