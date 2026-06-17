package media

// AudioInput is the handler-facing representation of an uploaded or recorded
// audio clip.
type AudioInput struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
	Path        string `json:"path"`
	URL         string `json:"url,omitempty"`
}

// AudioOutput is the handler-facing representation of a generated audio clip.
type AudioOutput struct {
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	Path        string `json:"path"`
}

// AudioAsset is the browser-safe representation of a stored audio clip.
type AudioAsset struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
	URL         string `json:"url"`
}
