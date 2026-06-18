package media

// AudioInput is the handler-facing representation of an uploaded or recorded audio clip.
type AudioInput struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
	Path        string `json:"path"`
	URL         string `json:"url,omitempty"`
}

// FileInput is the handler-facing representation of an uploaded file.
type FileInput struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
	Path        string `json:"path"`
	URL         string `json:"url,omitempty"`
}

// ImageInput is the handler-facing representation of an uploaded image.
type ImageInput struct {
	FileInput
}

// FileLikeOutput is the common handler-facing payload for generated file-like outputs.
type FileLikeOutput struct {
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	Path        string `json:"path"`
}

// AudioOutput is the handler-facing representation of a generated audio clip.
type AudioOutput = FileLikeOutput

// FileOutput is the handler-facing representation of a generated file.
type FileOutput = FileLikeOutput

// ImageOutput is the handler-facing representation of a generated image.
type ImageOutput = FileLikeOutput

// AudioAsset is the browser-safe representation of a stored file-like asset.
type AudioAsset struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
	URL         string `json:"url"`
}

// FileAsset is the browser-safe representation of a stored file-like asset.
type FileAsset = AudioAsset

// ImageAsset is the browser-safe representation of a stored image-like asset.
type ImageAsset = AudioAsset
