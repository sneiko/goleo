package goleo

import (
	"net/http"

	"github.com/sneiko/goleo/adapter"
)

type HTTPAdapterOptions struct {
	Client *http.Client
	URL    string
}

func HTTPAdapter(options HTTPAdapterOptions) *HandlerBinding {
	return adapter.HTTP(adapter.HTTPOptions{
		Client: options.Client,
		URL:    options.URL,
	})
}

func ProcessAdapter(command string, args ...string) *HandlerBinding {
	return adapter.Process(command, args...)
}

type OpenAICompatibleOptions = adapter.OpenAICompatibleOptions

type OllamaOptions = adapter.OllamaOptions

func OllamaAdapter(options OllamaOptions) *HandlerBinding {
	return adapter.Ollama(options)
}

func OllamaStreamAdapter(options OllamaOptions) *HandlerBinding {
	return adapter.OllamaStream(options)
}

func OpenAICompatibleAdapter(options OpenAICompatibleOptions) *HandlerBinding {
	return adapter.OpenAICompatible(options)
}

func OpenAICompatibleStreamAdapter(options OpenAICompatibleOptions) *HandlerBinding {
	return adapter.OpenAICompatibleStream(options)
}
