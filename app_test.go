package goleo_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sneiko/goleo"
)

func TestUpdateHelpersBuildPointerFields(t *testing.T) {
	t.Parallel()

	disabled := goleo.Disabled(true)
	if disabled.Disabled == nil || *disabled.Disabled != true {
		t.Fatalf("Disabled helper = %#v, want pointer true", disabled.Disabled)
	}

	visible := goleo.Visible(false)
	if visible.Visible == nil || *visible.Visible != false {
		t.Fatalf("Visible helper = %#v, want pointer false", visible.Visible)
	}

	label := goleo.Label("Run")
	if label.Label == nil || *label.Label != "Run" {
		t.Fatalf("Label helper = %#v, want Run", label.Label)
	}

	choices := goleo.Choices("a", "b")
	if !reflect.DeepEqual(choices.Choices, []string{"a", "b"}) {
		t.Fatalf("Choices helper = %#v, want [a b]", choices.Choices)
	}
}

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

func TestAudioComponentCanBeUsedInSchema(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	app.Interface(
		goleo.Handler(func(input goleo.AudioInput) (goleo.AudioOutput, error) {
			return goleo.AudioOutput{}, nil
		}),
		goleo.Inputs(goleo.Audio("Prompt audio", goleo.WithAccept("audio/*"))),
		goleo.Outputs(goleo.Audio("Reply audio")),
	)

	schema := app.Schema()

	if got := schema.Interfaces[0].Inputs[0].Type; got != "audio" {
		t.Fatalf("input type = %q, want audio", got)
	}
	if got := schema.Interfaces[0].Outputs[0].Type; got != "audio" {
		t.Fatalf("output type = %q, want audio", got)
	}
	if got := schema.Interfaces[0].Inputs[0].Props["accept"]; got != "audio/*" {
		t.Fatalf("accept = %#v, want audio/*", got)
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

func TestUploadEndpointStoresAssetAndServesIt(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	server := httptest.NewServer(app.Handler())
	defer server.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "prompt.wav")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte("wave-data")); err != nil {
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

	var uploaded struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Size        int64  `json:"size"`
		ContentType string `json:"content_type"`
		URL         string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&uploaded); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if uploaded.ID == "" {
		t.Fatal("uploaded id is empty")
	}
	if uploaded.URL == "" {
		t.Fatal("uploaded url is empty")
	}

	assetResp, err := http.Get(server.URL + uploaded.URL)
	if err != nil {
		t.Fatalf("get asset: %v", err)
	}
	defer assetResp.Body.Close()

	if assetResp.StatusCode != http.StatusOK {
		t.Fatalf("asset status = %d, want %d", assetResp.StatusCode, http.StatusOK)
	}

	var assetBody bytes.Buffer
	if _, err := assetBody.ReadFrom(assetResp.Body); err != nil {
		t.Fatalf("read asset body: %v", err)
	}
	if got := assetBody.String(); got != "wave-data" {
		t.Fatalf("asset body = %q, want wave-data", got)
	}
	if got := assetResp.Header.Get("Content-Type"); got == "" {
		t.Fatal("asset content-type is empty")
	}
}

func TestPredictEndpointHydratesAudioInputAndDehydratesAudioOutput(t *testing.T) {
	t.Parallel()

	outputDir := t.TempDir()
	app := goleo.New()
	app.Interface(
		goleo.Handler(func(input goleo.AudioInput) (goleo.AudioOutput, error) {
			payload, err := os.ReadFile(input.Path)
			if err != nil {
				return goleo.AudioOutput{}, err
			}

			outputPath := filepath.Join(outputDir, "reply.wav")
			if err := os.WriteFile(outputPath, bytes.ToUpper(payload), 0o600); err != nil {
				return goleo.AudioOutput{}, err
			}

			return goleo.AudioOutput{
				Name:        "reply.wav",
				ContentType: "audio/wav",
				Path:        outputPath,
			}, nil
		}),
		goleo.Inputs(goleo.Audio("Prompt audio", goleo.WithAccept("audio/*"))),
		goleo.Outputs(goleo.Audio("Reply audio")),
	)

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	var uploadBody bytes.Buffer
	writer := multipart.NewWriter(&uploadBody)
	part, err := writer.CreateFormFile("file", "prompt.wav")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte("voice")); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	uploadResp, err := http.Post(server.URL+"/api/upload", writer.FormDataContentType(), &uploadBody)
	if err != nil {
		t.Fatalf("post upload: %v", err)
	}
	defer uploadResp.Body.Close()

	var uploaded map[string]any
	if err := json.NewDecoder(uploadResp.Body).Decode(&uploaded); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}

	predictBody, err := json.Marshal(map[string]any{
		"interface_id": "interface-1",
		"data":         []any{uploaded},
	})
	if err != nil {
		t.Fatalf("marshal predict body: %v", err)
	}

	resp, err := http.Post(server.URL+"/api/predict", "application/json", bytes.NewReader(predictBody))
	if err != nil {
		t.Fatalf("post predict: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var got struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got.Data) != 1 {
		t.Fatalf("len(data) = %d, want 1", len(got.Data))
	}
	if _, ok := got.Data[0]["url"].(string); !ok {
		t.Fatalf("audio output = %#v, want url", got.Data[0])
	}

	assetResp, err := http.Get(server.URL + got.Data[0]["url"].(string))
	if err != nil {
		t.Fatalf("get asset: %v", err)
	}
	defer assetResp.Body.Close()

	var assetBody bytes.Buffer
	if _, err := assetBody.ReadFrom(assetResp.Body); err != nil {
		t.Fatalf("read asset body: %v", err)
	}
	if got := assetBody.String(); got != "VOICE" {
		t.Fatalf("asset body = %q, want VOICE", got)
	}
}

func TestPredictEndpointHydratesFileAndImageInputsAndDehydratesFileAndImageOutputs(t *testing.T) {
	t.Parallel()

	outputDir := t.TempDir()
	app := goleo.New()
	app.Interface(
		goleo.Handler(func(fileInput goleo.FileInput, imageInput goleo.ImageInput) (goleo.FileOutput, goleo.ImageOutput, error) {
			filePayload, err := os.ReadFile(fileInput.Path)
			if err != nil {
				return goleo.FileOutput{}, goleo.ImageOutput{}, err
			}
			imagePayload, err := os.ReadFile(imageInput.Path)
			if err != nil {
				return goleo.FileOutput{}, goleo.ImageOutput{}, err
			}

			fileOutputPath := filepath.Join(outputDir, "reply.txt")
			if err := os.WriteFile(fileOutputPath, bytes.ToUpper(filePayload), 0o600); err != nil {
				return goleo.FileOutput{}, goleo.ImageOutput{}, err
			}

			imageOutputPath := filepath.Join(outputDir, "reply-image.txt")
			if err := os.WriteFile(imageOutputPath, append(imagePayload, []byte("-img")...), 0o600); err != nil {
				return goleo.FileOutput{}, goleo.ImageOutput{}, err
			}

			return goleo.FileOutput{
					Name:        "reply.txt",
					ContentType: fileInput.ContentType,
					Path:        fileOutputPath,
				}, goleo.ImageOutput{
					Name:        "reply-image.txt",
					ContentType: imageInput.ContentType,
					Path:        imageOutputPath,
				}, nil
		}),
		goleo.Inputs(goleo.File("Input file"), goleo.Image("Input image")),
		goleo.Outputs(goleo.File("Reply file"), goleo.Image("Reply image")),
	)

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	uploadBody := func(filename, data string) string {
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		part, err := writer.CreateFormFile("file", filename)
		if err != nil {
			t.Fatalf("create form file %q: %v", filename, err)
		}
		if _, err := part.Write([]byte(data)); err != nil {
			t.Fatalf("write form file %q: %v", filename, err)
		}
		if err := writer.Close(); err != nil {
			t.Fatalf("close form writer %q: %v", filename, err)
		}
		req, err := http.NewRequest(http.MethodPost, server.URL+"/api/upload", &buf)
		if err != nil {
			t.Fatalf("create upload request: %v", err)
		}
		req.Header.Set("Content-Type", writer.FormDataContentType())
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("upload %q: %v", filename, err)
		}
		defer resp.Body.Close()

		var uploaded map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&uploaded); err != nil {
			t.Fatalf("decode upload response %q: %v", filename, err)
		}
		result, ok := uploaded["id"].(string)
		if !ok || result == "" {
			t.Fatalf("upload response %q: missing id", filename)
		}
		return result
	}

	fileID := uploadBody("prompt.txt", "alpha")
	imageID := uploadBody("pic.txt", "image")

	predictBody, err := json.Marshal(map[string]any{
		"interface_id": "interface-1",
		"data": []any{
			map[string]any{"id": fileID},
			map[string]any{"id": imageID, "name": "pic.txt"},
		},
	})
	if err != nil {
		t.Fatalf("marshal predict body: %v", err)
	}

	resp, err := http.Post(server.URL+"/api/predict", "application/json", bytes.NewReader(predictBody))
	if err != nil {
		t.Fatalf("post predict: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var got struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode predict response: %v", err)
	}
	if len(got.Data) != 2 {
		t.Fatalf("len(data) = %d, want 2", len(got.Data))
	}

	fileURL, _ := got.Data[0]["url"].(string)
	imageURL, _ := got.Data[1]["url"].(string)
	if fileURL == "" || imageURL == "" {
		t.Fatalf("missing asset urls in response: %#v", got.Data)
	}

	fileAssetResp, err := http.Get(server.URL + fileURL)
	if err != nil {
		t.Fatalf("get file asset: %v", err)
	}
	defer fileAssetResp.Body.Close()
	var fileAssetBody bytes.Buffer
	if _, err := fileAssetBody.ReadFrom(fileAssetResp.Body); err != nil {
		t.Fatalf("read file asset: %v", err)
	}
	if fileAssetBody.String() != "ALPHA" {
		t.Fatalf("file asset = %q, want ALPHA", fileAssetBody.String())
	}

	imageAssetResp, err := http.Get(server.URL + imageURL)
	if err != nil {
		t.Fatalf("get image asset: %v", err)
	}
	defer imageAssetResp.Body.Close()
	var imageAssetBody bytes.Buffer
	if _, err := imageAssetBody.ReadFrom(imageAssetResp.Body); err != nil {
		t.Fatalf("read image asset: %v", err)
	}
	if imageAssetBody.String() != "image-img" {
		t.Fatalf("image asset = %q, want image-img", imageAssetBody.String())
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
	if !strings.Contains(got, `"status":"running"`) {
		t.Fatalf("stream body = %q, want running status event", got)
	}
	if !strings.Contains(got, `"status":"done"`) {
		t.Fatalf("stream body = %q, want done status event", got)
	}
	if !strings.Contains(got, `"data":"one"`) || !strings.Contains(got, `"data":"two"`) {
		t.Fatalf("stream body = %q, want two chunk payloads", got)
	}
}

func TestPredictEndpointQueueLimitReturnsError(t *testing.T) {
	app := goleo.New()
	app.ConfigureQueue(1, 0)
	app.Interface(
		goleo.Handler(func(input string) (string, error) {
			time.Sleep(300 * time.Millisecond)
			return strings.ToUpper(input), nil
		}),
		goleo.Inputs(goleo.Textbox("Prompt")),
		goleo.Outputs(goleo.Textbox("Result")),
	)

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	requestBody := func(value string) io.Reader {
		return strings.NewReader(`{"interface_id":"interface-1","data":["` + value + `"]}`)
	}

	firstDone := make(chan struct{})
	var firstErr error
	go func() {
		_, err := http.Post(server.URL+"/api/predict", "application/json", requestBody("hello"))
		if err != nil {
			firstErr = err
		}
		close(firstDone)
	}()

	time.Sleep(20 * time.Millisecond)
	resp, err := http.Post(server.URL+"/api/predict", "application/json", requestBody("world"))
	if err != nil {
		t.Fatalf("post second predict: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusTooManyRequests)
	}

	var got struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode queue response: %v", err)
	}
	if got.Error.Code != "queue_full" {
		t.Fatalf("error code = %q, want queue_full", got.Error.Code)
	}

	<-firstDone
	if firstErr != nil {
		t.Fatalf("first predict request failed: %v", firstErr)
	}
}

func TestStreamEndpointSupportsCancellation(t *testing.T) {
	app := goleo.New()
	var cancelled int32
	app.Chat(goleo.StreamHandler(func(ctx context.Context, prompt string, emit goleo.EmitFunc) error {
		for i := 0; i < 100; i++ {
			select {
			case <-ctx.Done():
				atomic.StoreInt32(&cancelled, 1)
				return ctx.Err()
			default:
			}

			emit(strings.Repeat("x", i+1))
			time.Sleep(20 * time.Millisecond)
		}

		return nil
	}))

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	requestBody := strings.NewReader(`{"interface_id":"chat-1","data":["hello"]}`)
	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/stream", requestBody)
	if err != nil {
		t.Fatalf("create stream request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "stream-cancel-test")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do stream request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stream status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	time.Sleep(80 * time.Millisecond)
	cancelResp, err := http.Post(server.URL+"/api/cancel", "application/json", strings.NewReader(`{"request_id":"stream-cancel-test"}`))
	if err != nil {
		t.Fatalf("post cancel: %v", err)
	}
	defer cancelResp.Body.Close()

	if cancelResp.StatusCode != http.StatusOK {
		t.Fatalf("cancel status = %d, want %d", cancelResp.StatusCode, http.StatusOK)
	}

	_, _ = io.Copy(io.Discard, resp.Body)

	if atomic.LoadInt32(&cancelled) == 0 {
		t.Fatalf("stream handler was not cancelled")
	}
}
