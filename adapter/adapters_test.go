package adapter

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestProcessAdapterReturnsStdout(t *testing.T) {
	t.Setenv("GOLEO_PROCESS_HELPER", "1")

	binding := Process(os.Args[0], "-test.run=TestProcessAdapterHelper", "--", "echo")
	result, err := binding.Invoke(context.Background(), []any{"hello", "world"})
	if err != nil {
		t.Fatalf("invoke process adapter: %v", err)
	}

	if len(result) != 1 || result[0] != "hello\nworld" {
		t.Fatalf("result = %#v, want [hello\\nworld]", result)
	}
}

func TestProcessAdapterIncludesStderrOnFailure(t *testing.T) {
	t.Setenv("GOLEO_PROCESS_HELPER", "1")

	binding := Process(os.Args[0], "-test.run=TestProcessAdapterHelper", "--", "fail")
	_, err := binding.Invoke(context.Background(), []any{"hello"})
	if err == nil {
		t.Fatal("invoke process adapter error is nil")
	}

	got := err.Error()
	if !strings.Contains(got, "process failed") {
		t.Fatalf("error = %q, want process failed context", got)
	}
	if !strings.Contains(got, "helper failure") {
		t.Fatalf("error = %q, want stderr output", got)
	}
}

func TestOpenAICompatibleAdapterCallsChatCompletions(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %q, want /v1/chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("authorization = %q, want bearer token", got)
		}

		var body struct {
			Model    string              `json:"model"`
			Messages []map[string]string `json:"messages"`
			Stream   bool                `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.Model != "test-model" {
			t.Fatalf("model = %q, want test-model", body.Model)
		}
		if body.Stream {
			t.Fatal("stream = true, want false")
		}
		if len(body.Messages) != 1 || body.Messages[0]["content"] != "hello" {
			t.Fatalf("messages = %#v, want user hello", body.Messages)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hi there"}}]}`))
	}))
	defer server.Close()

	binding := OpenAICompatible(OpenAICompatibleOptions{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "test-model",
	})
	got, err := binding.Invoke(context.Background(), []any{"hello"})
	if err != nil {
		t.Fatalf("Invoke error = %v", err)
	}
	if !reflect.DeepEqual(got, []any{"hi there"}) {
		t.Fatalf("result = %#v, want [hi there]", got)
	}
}

func TestOpenAICompatibleStreamAdapterEmitsDeltas(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %q, want /v1/chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Accept"); got != "text/event-stream" {
			t.Fatalf("accept = %q, want text/event-stream", got)
		}

		var body struct {
			Model    string              `json:"model"`
			Messages []map[string]string `json:"messages"`
			Stream   bool                `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.Model != "test-model" {
			t.Fatalf("model = %q, want test-model", body.Model)
		}
		if !body.Stream {
			t.Fatal("stream = false, want true")
		}
		if len(body.Messages) != 1 || body.Messages[0]["content"] != "hello" {
			t.Fatalf("messages = %#v, want user hello", body.Messages)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hel\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"lo\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	binding := OpenAICompatibleStream(OpenAICompatibleOptions{
		BaseURL: server.URL,
		Model:   "test-model",
	})

	var got []any
	err := binding.Stream(context.Background(), []any{"hello"}, func(value any) {
		got = append(got, value)
	})
	if err != nil {
		t.Fatalf("Stream error = %v", err)
	}
	if !reflect.DeepEqual(got, []any{"Hel", "lo"}) {
		t.Fatalf("emitted = %#v, want [Hel lo]", got)
	}
}

func TestOllamaAdapterUsesOpenAICompatibleEndpoint(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %q, want /v1/chat/completions", r.URL.Path)
		}

		var body struct {
			Model    string              `json:"model"`
			Messages []map[string]string `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.Model != "llama3.2" {
			t.Fatalf("model = %q, want llama3.2", body.Model)
		}
		if len(body.Messages) != 1 || body.Messages[0]["content"] != "hello" {
			t.Fatalf("messages = %#v, want user hello", body.Messages)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ollama response"}}]}`))
	}))
	defer server.Close()

	binding := Ollama(OllamaOptions{BaseURL: server.URL, Model: "llama3.2"})
	got, err := binding.Invoke(context.Background(), []any{"hello"})
	if err != nil {
		t.Fatalf("Invoke error = %v", err)
	}
	if !reflect.DeepEqual(got, []any{"ollama response"}) {
		t.Fatalf("result = %#v, want [ollama response]", got)
	}
}

func TestOllamaOptionsDefaultBaseURL(t *testing.T) {
	t.Parallel()

	got := openAIOptionsFromOllama(OllamaOptions{Model: "llama3.2"})
	if got.BaseURL != defaultOllamaBaseURL {
		t.Fatalf("BaseURL = %q, want %q", got.BaseURL, defaultOllamaBaseURL)
	}
	if got.Model != "llama3.2" {
		t.Fatalf("Model = %q, want llama3.2", got.Model)
	}
}

func TestOpenAICompatibleStreamAdapterRejectsInvalidChunk(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: not-json\n\n"))
	}))
	defer server.Close()

	binding := OpenAICompatibleStream(OpenAICompatibleOptions{
		BaseURL: server.URL,
		Model:   "test-model",
	})

	err := binding.Stream(context.Background(), []any{"hello"}, func(any) {})
	if err == nil {
		t.Fatal("Stream error is nil, want JSON decode error")
	}
}

func TestProcessAdapterHelper(t *testing.T) {
	if os.Getenv("GOLEO_PROCESS_HELPER") != "1" {
		return
	}

	mode := os.Args[len(os.Args)-1]
	switch mode {
	case "echo":
		payload, err := io.ReadAll(os.Stdin)
		if err != nil {
			_, _ = os.Stderr.WriteString(err.Error())
			os.Exit(2)
		}
		_, _ = os.Stdout.Write(payload)
		os.Exit(0)
	case "fail":
		_, _ = os.Stderr.WriteString("helper failure")
		os.Exit(7)
	default:
		_, _ = os.Stderr.WriteString("unknown helper mode")
		os.Exit(2)
	}
}
