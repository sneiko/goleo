package goleo_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sneiko/goleo"
)

func TestInterfaceSchemaIncludesComponents(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	app.Interface(
		goleo.Handler(func(input string) (string, error) {
			return "Hello " + input, nil
		}),
		goleo.Inputs(goleo.Textbox("Prompt")),
		goleo.Outputs(goleo.Textbox("Result")),
	)

	schema := app.Schema()

	if schema.Version == "" {
		t.Fatal("schema version is empty")
	}
	if len(schema.Interfaces) != 1 {
		t.Fatalf("len(schema.Interfaces) = %d, want 1", len(schema.Interfaces))
	}

	if got := schema.Interfaces[0].Inputs[0].Type; got != "textbox" {
		t.Fatalf("input type = %q, want textbox", got)
	}
	if got := schema.Interfaces[0].Inputs[0].Label; got != "Prompt" {
		t.Fatalf("input label = %q, want Prompt", got)
	}
	if got := schema.Interfaces[0].Outputs[0].Label; got != "Result" {
		t.Fatalf("output label = %q, want Result", got)
	}
}

func TestComponentTypedOptionsAreIncludedInSchema(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	app.Interface(
		goleo.Handler(func(input string, value float64) (string, error) {
			return input, nil
		}),
		goleo.Inputs(
			goleo.Textbox("Prompt", goleo.WithPlaceholder("Ask something"), goleo.WithDefault("Hello"), goleo.WithRows(3)),
			goleo.Slider("Temperature", goleo.WithMin(0), goleo.WithMax(2), goleo.WithStep(0.1), goleo.WithDefault(0.7)),
			goleo.File("Image", goleo.WithAccept("image/*"), goleo.WithMultiple(false), goleo.WithVisible(true)),
		),
		goleo.Outputs(goleo.Textbox("Result", goleo.WithDisabled(true))),
	)

	schema := app.Schema()
	inputs := schema.Interfaces[0].Inputs
	if inputs[0].Props["placeholder"] != "Ask something" {
		t.Fatalf("placeholder = %#v, want Ask something", inputs[0].Props["placeholder"])
	}
	if inputs[0].Props["default"] != "Hello" {
		t.Fatalf("textbox default = %#v, want Hello", inputs[0].Props["default"])
	}
	if inputs[0].Props["rows"] != 3 {
		t.Fatalf("rows = %#v, want 3", inputs[0].Props["rows"])
	}
	if inputs[1].Props["min"] != float64(0) || inputs[1].Props["max"] != float64(2) || inputs[1].Props["step"] != 0.1 {
		t.Fatalf("slider props = %#v, want min/max/step", inputs[1].Props)
	}
	if inputs[1].Props["default"] != 0.7 {
		t.Fatalf("slider default = %#v, want 0.7", inputs[1].Props["default"])
	}
	if inputs[2].Props["accept"] != "image/*" || inputs[2].Props["multiple"] != false || inputs[2].Props["visible"] != true {
		t.Fatalf("file props = %#v, want accept/multiple/visible", inputs[2].Props)
	}
	if schema.Interfaces[0].Outputs[0].Props["disabled"] != true {
		t.Fatalf("disabled = %#v, want true", schema.Interfaces[0].Outputs[0].Props["disabled"])
	}
}

func TestCustomComponentCanBeUsedInSchema(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	app.Interface(
		goleo.Handler(func(input string) (string, error) {
			return input, nil
		}),
		goleo.Inputs(goleo.CustomComponent("audio", "Audio", goleo.WithProp("source", "upload"))),
		goleo.Outputs(goleo.JSON("Metadata")),
	)

	schema := app.Schema()

	input := schema.Interfaces[0].Inputs[0]
	if input.Type != "audio" {
		t.Fatalf("input type = %q, want audio", input.Type)
	}
	if input.Props["source"] != "upload" {
		t.Fatalf("source prop = %#v, want upload", input.Props["source"])
	}
}

func TestServerUsesLaunchOptions(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	server := app.Server(goleo.LaunchOptions{
		Addr:              ":9000",
		ReadTimeout:       time.Second,
		ReadHeaderTimeout: 2 * time.Second,
		WriteTimeout:      3 * time.Second,
		IdleTimeout:       4 * time.Second,
	})

	if server.Addr != ":9000" {
		t.Fatalf("server.Addr = %q, want :9000", server.Addr)
	}
	if server.Handler == nil {
		t.Fatal("server.Handler is nil")
	}
	if server.ReadTimeout != time.Second {
		t.Fatalf("ReadTimeout = %s, want 1s", server.ReadTimeout)
	}
	if server.ReadHeaderTimeout != 2*time.Second {
		t.Fatalf("ReadHeaderTimeout = %s, want 2s", server.ReadHeaderTimeout)
	}
	if server.WriteTimeout != 3*time.Second {
		t.Fatalf("WriteTimeout = %s, want 3s", server.WriteTimeout)
	}
	if server.IdleTimeout != 4*time.Second {
		t.Fatalf("IdleTimeout = %s, want 4s", server.IdleTimeout)
	}
}

func TestServerUsesDefaultAddr(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	server := app.Server(goleo.LaunchOptions{})

	if server.Addr != ":7860" {
		t.Fatalf("server.Addr = %q, want :7860", server.Addr)
	}
}

func TestLaunchContextShutsDownOnCancel(t *testing.T) {
	app := goleo.New()
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- app.LaunchContext(ctx, goleo.LaunchOptions{
			Addr:            "127.0.0.1:0",
			ShutdownTimeout: time.Second,
		})
	}()

	time.Sleep(25 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("LaunchContext error = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("LaunchContext did not stop after context cancellation")
	}
}

func TestPredictEndpointInvokesInterfaceHandler(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	app.Interface(
		goleo.Handler(func(input string) (string, error) {
			return strings.ToUpper(input), nil
		}),
		goleo.Inputs(goleo.Textbox("Prompt")),
		goleo.Outputs(goleo.Textbox("Result")),
	)

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	body := strings.NewReader(`{"interface_id":"interface-1","data":["hello"]}`)
	resp, err := http.Post(server.URL+"/api/predict", "application/json", body)
	if err != nil {
		t.Fatalf("post predict: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var got struct {
		Data []any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got.Data) != 1 || got.Data[0] != "HELLO" {
		t.Fatalf("data = %#v, want [HELLO]", got.Data)
	}
}

func TestRootServesEmbeddedFrontend(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	server := httptest.NewServer(app.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("get root: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body bytes.Buffer
	if _, err := body.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(body.String(), "<title>Goleo</title>") {
		t.Fatalf("root body does not contain Goleo title: %q", body.String())
	}
}

func TestUploadEndpointStoresFileMetadata(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	server := httptest.NewServer(app.Handler())
	defer server.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "prompt.txt")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte("hello")); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	resp, err := http.Post(server.URL+"/api/upload", writer.FormDataContentType(), &body)
	if err != nil {
		t.Fatalf("post upload: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var got struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Name != "prompt.txt" || got.Size != 5 {
		t.Fatalf("upload = %#v, want name prompt.txt and size 5", got)
	}
}

func TestCustomLoggerReceivesRequestLogs(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	app := goleo.New(goleo.WithLogger(logger))

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/schema")
	if err != nil {
		t.Fatalf("get schema: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	requestID := resp.Header.Get("X-Request-ID")
	if requestID == "" {
		t.Fatal("X-Request-ID header is empty")
	}

	got := logs.String()
	if !strings.Contains(got, "http request completed") {
		t.Fatalf("logs = %q, want request completion log", got)
	}
	if !strings.Contains(got, `"path":"/api/schema"`) {
		t.Fatalf("logs = %q, want schema path", got)
	}
	if !strings.Contains(got, `"request_id":"`+requestID+`"`) {
		t.Fatalf("logs = %q, want request_id %q", got, requestID)
	}
}

func TestPredictEndpointRecoversHandlerPanic(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	app := goleo.New(goleo.WithLogger(logger))
	app.Interface(
		goleo.Handler(func(string) string {
			panic("secret panic value")
		}),
		goleo.Inputs(goleo.Textbox("Prompt")),
		goleo.Outputs(goleo.Textbox("Result")),
	)

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	body := strings.NewReader(`{"interface_id":"interface-1","data":["hello"]}`)
	resp, err := http.Post(server.URL+"/api/predict", "application/json", body)
	if err != nil {
		t.Fatalf("post predict: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}

	var got struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Error.Code != "internal_error" {
		t.Fatalf("error code = %q, want internal_error", got.Error.Code)
	}
	if got.Error.Message != "handler panic" {
		t.Fatalf("error message = %q, want handler panic", got.Error.Message)
	}

	logText := logs.String()
	if !strings.Contains(logText, "predict handler panicked") {
		t.Fatalf("logs = %q, want panic log", logText)
	}
	if strings.Contains(logText, "secret panic value") {
		t.Fatalf("logs = %q, should not expose panic value", logText)
	}
}

func TestPredictEndpointReturnsStructuredNotFoundError(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	server := httptest.NewServer(app.Handler())
	defer server.Close()

	body := strings.NewReader(`{"interface_id":"missing","data":[]}`)
	resp, err := http.Post(server.URL+"/api/predict", "application/json", body)
	if err != nil {
		t.Fatalf("post predict: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}

	var got struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Error.Code != "not_found" {
		t.Fatalf("error code = %q, want not_found", got.Error.Code)
	}
	if got.Error.Message != `interface "missing" not found` {
		t.Fatalf("error message = %q, want interface missing not found", got.Error.Message)
	}
}

func TestServerPreservesIncomingRequestID(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	app := goleo.New(goleo.WithLogger(logger))

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	request, err := http.NewRequest(http.MethodGet, server.URL+"/api/schema", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	request.Header.Set("X-Request-ID", "test-request-id")

	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("get schema: %v", err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("X-Request-ID"); got != "test-request-id" {
		t.Fatalf("X-Request-ID = %q, want test-request-id", got)
	}
	if got := logs.String(); !strings.Contains(got, `"request_id":"test-request-id"`) {
		t.Fatalf("logs = %q, want incoming request_id", got)
	}
}

func TestStreamEndpointEmitsServerSentEvents(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	app.Chat(goleo.StreamHandler(func(input string, emit goleo.EmitFunc) error {
		emit("one")
		emit("two")
		return nil
	}))

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	body := strings.NewReader(`{"interface_id":"chat-1","data":["go"]}`)
	resp, err := http.Post(server.URL+"/api/stream", "application/json", body)
	if err != nil {
		t.Fatalf("post stream: %v", err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("content-type = %q, want text/event-stream", got)
	}

	var responseBody bytes.Buffer
	if _, err := responseBody.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read response: %v", err)
	}
	got := responseBody.String()
	if !strings.Contains(got, "data: one\n\n") || !strings.Contains(got, "data: two\n\n") {
		t.Fatalf("stream body = %q, want two data events", got)
	}
}
