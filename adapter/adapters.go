package adapter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"strings"

	"github.com/sneiko/goleo/runtime"
)

const (
	defaultOllamaBaseURL  = "http://localhost:11434/v1"
	maxProcessErrorOutput = 4096
	maxStreamLineSize     = 1024 * 1024
)

type predictResponse struct {
	Data []any `json:"data"`
}

// HTTPOptions configures a generic JSON-over-HTTP model adapter.
type HTTPOptions struct {
	Client *http.Client
	URL    string
}

// HTTP calls an external endpoint with {"data": [...]} and expects the same response shape.
func HTTP(options HTTPOptions) *runtime.HandlerBinding {
	client := options.Client
	if client == nil {
		client = http.DefaultClient
	}

	return runtime.Callable(func(ctx context.Context, data []any) ([]any, error) {
		if options.URL == "" {
			return nil, errors.New("http adapter URL is required")
		}

		payload, err := json.Marshal(predictResponse{Data: data})
		if err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, options.URL, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			return nil, errors.New(resp.Status)
		}

		var result predictResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, err
		}

		return result.Data, nil
	})
}

// Process runs a local command with inputs joined by newlines and returns stdout.
func Process(command string, args ...string) *runtime.HandlerBinding {
	return runtime.Callable(func(ctx context.Context, data []any) ([]any, error) {
		if command == "" {
			return nil, errors.New("process command is required")
		}

		lines := make([]string, 0, len(data))
		for _, value := range data {
			lines = append(lines, strings.TrimSpace(toString(value)))
		}

		cmd := exec.CommandContext(ctx, command, args...)
		cmd.Stdin = strings.NewReader(strings.Join(lines, "\n"))
		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		output, err := cmd.Output()
		if err != nil {
			return nil, processError(err, stderr.String())
		}

		return []any{strings.TrimSpace(string(output))}, nil
	})
}

// OpenAICompatibleOptions configures a minimal OpenAI-compatible chat adapter.
type OpenAICompatibleOptions struct {
	Client  *http.Client
	BaseURL string
	APIKey  string
	Model   string
}

// OllamaOptions configures a local Ollama chat adapter.
type OllamaOptions struct {
	Client  *http.Client
	BaseURL string
	Model   string
}

// Ollama calls the OpenAI-compatible Ollama endpoint and returns the first assistant message.
func Ollama(options OllamaOptions) *runtime.HandlerBinding {
	return OpenAICompatible(openAIOptionsFromOllama(options))
}

// OllamaStream calls the OpenAI-compatible Ollama endpoint with stream=true and emits content deltas.
func OllamaStream(options OllamaOptions) *runtime.HandlerBinding {
	return OpenAICompatibleStream(openAIOptionsFromOllama(options))
}

// OpenAICompatible calls /v1/chat/completions and returns the first assistant message.
func OpenAICompatible(options OpenAICompatibleOptions) *runtime.HandlerBinding {
	client := options.Client
	if client == nil {
		client = http.DefaultClient
	}

	return runtime.Callable(func(ctx context.Context, data []any) ([]any, error) {
		prompt := ""
		if len(data) > 0 {
			prompt = toString(data[0])
		}

		req, err := newOpenAICompatibleRequest(ctx, options, prompt, false)
		if err != nil {
			return nil, err
		}

		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			return nil, errors.New(resp.Status)
		}

		var response struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			return nil, err
		}
		if len(response.Choices) == 0 {
			return nil, errors.New("openai-compatible response has no choices")
		}

		return []any{response.Choices[0].Message.Content}, nil
	})
}

// OpenAICompatibleStream calls /v1/chat/completions with stream=true and emits content deltas.
func OpenAICompatibleStream(options OpenAICompatibleOptions) *runtime.HandlerBinding {
	client := options.Client
	if client == nil {
		client = http.DefaultClient
	}

	return runtime.StreamHandler(func(ctx context.Context, prompt string, emit runtime.EmitFunc) error {
		req, err := newOpenAICompatibleRequest(ctx, options, prompt, true)
		if err != nil {
			return err
		}

		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			return errors.New(resp.Status)
		}

		return readOpenAICompatibleStream(resp, emit)
	})
}

func openAIOptionsFromOllama(options OllamaOptions) OpenAICompatibleOptions {
	baseURL := options.BaseURL
	if baseURL == "" {
		baseURL = defaultOllamaBaseURL
	}

	return OpenAICompatibleOptions{
		Client:  options.Client,
		BaseURL: baseURL,
		Model:   options.Model,
	}
}

func newOpenAICompatibleRequest(ctx context.Context, options OpenAICompatibleOptions, prompt string, stream bool) (*http.Request, error) {
	if options.BaseURL == "" {
		return nil, errors.New("openai-compatible base URL is required")
	}
	if options.Model == "" {
		return nil, errors.New("openai-compatible model is required")
	}

	requestBody := map[string]any{
		"model": options.Model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	if stream {
		requestBody["stream"] = true
	}

	payload, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}

	url := strings.TrimRight(options.BaseURL, "/") + "/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	if options.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+options.APIKey)
	}

	return req, nil
}

func readOpenAICompatibleStream(resp *http.Response, emit runtime.EmitFunc) error {
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), maxStreamLineSize)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			return nil
		}

		content, err := openAICompatibleStreamContent(data)
		if err != nil {
			return err
		}
		if content != "" {
			emit(content)
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	return nil
}

func openAICompatibleStreamContent(data string) (string, error) {
	var chunk struct {
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return "", err
	}
	if len(chunk.Choices) == 0 {
		return "", nil
	}
	if chunk.Choices[0].Delta.Content != "" {
		return chunk.Choices[0].Delta.Content, nil
	}

	return chunk.Choices[0].Message.Content, nil
}

func processError(err error, stderr string) error {
	stderr = strings.TrimSpace(stderr)
	if stderr == "" {
		return fmt.Errorf("process failed: %w", err)
	}

	if len(stderr) > maxProcessErrorOutput {
		stderr = stderr[:maxProcessErrorOutput] + "..."
	}

	return fmt.Errorf("process failed: %w: %s", err, stderr)
}

func toString(value any) string {
	text, ok := value.(string)
	if ok {
		return text
	}

	payload, err := json.Marshal(value)
	if err != nil {
		return ""
	}

	return string(payload)
}
