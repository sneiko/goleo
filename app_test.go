package goleo_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

func TestBlocksSchemaIncludesComponentsAndEvents(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	var prompt, run, out goleo.Component
	app.Blocks(func(blocks *goleo.Blocks) {
		prompt = blocks.Textbox("Prompt")
		run = blocks.Button("Run")
		out = blocks.Textbox("Result")

		run.Click(
			goleo.Handler(func(input string) (string, error) {
				return strings.ToUpper(input), nil
			}),
			goleo.Inputs(prompt),
			goleo.Outputs(out),
		)
	})

	schema := app.Schema()
	if len(schema.Interfaces) != 1 {
		t.Fatalf("len(schema.Interfaces) = %d, want 1", len(schema.Interfaces))
	}
	iface := schema.Interfaces[0]
	if iface.Kind != "blocks" {
		t.Fatalf("kind = %q, want blocks", iface.Kind)
	}
	if len(iface.Components) != 3 {
		t.Fatalf("len(components) = %d, want 3", len(iface.Components))
	}
	if len(iface.Events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(iface.Events))
	}
	if iface.Events[0].Trigger != "click" || iface.Events[0].Source != run.ID {
		t.Fatalf("event = %#v, want click from run", iface.Events[0])
	}
	if !reflect.DeepEqual(iface.Events[0].Inputs, []string{prompt.ID}) {
		t.Fatalf("event inputs = %#v, want prompt id", iface.Events[0].Inputs)
	}
	if !reflect.DeepEqual(iface.Events[0].Outputs, []string{out.ID}) {
		t.Fatalf("event outputs = %#v, want out id", iface.Events[0].Outputs)
	}
}

func TestBlocksSchemaComponentsDoNotRetainEventBinder(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	app.Blocks(func(blocks *goleo.Blocks) {
		prompt := blocks.Textbox("Prompt")
		run := blocks.Button("Run")
		run.Click(
			goleo.Handler(func(input string) (string, error) { return input, nil }),
			goleo.Inputs(prompt),
			goleo.Outputs(prompt),
		)
	})

	componentValue := reflect.ValueOf(app.Schema().Interfaces[0].Components[0])
	binder := componentValue.FieldByName("eventBinder")
	if binder.IsValid() && !binder.IsNil() {
		t.Fatalf("schema component retained event binder")
	}
}

func TestBlocksSchemaEventsDoNotRetainHandlers(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	app.Blocks(func(blocks *goleo.Blocks) {
		prompt := blocks.Textbox("Prompt")
		run := blocks.Button("Run")
		run.Click(
			goleo.Handler(func(input string) (string, error) { return input, nil }),
			goleo.Inputs(prompt),
			goleo.Outputs(prompt),
		)
	})

	if handler := app.Schema().Interfaces[0].Events[0].Handler; handler != nil {
		t.Fatalf("schema event retained handler: %#v", handler)
	}
}

func TestGetInterfaceReturnsDefensiveEventSlices(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	app.Blocks(func(blocks *goleo.Blocks) {
		prompt := blocks.Textbox("Prompt")
		run := blocks.Button("Run")
		run.Click(
			goleo.Handler(func(input string) (string, error) { return input, nil }),
			goleo.Inputs(prompt),
			goleo.Outputs(prompt),
		)
	})

	iface, ok := app.GetInterface("blocks-1")
	if !ok {
		t.Fatal("blocks interface not found")
	}
	iface.Events[0].Inputs[0] = "mutated"
	iface.Events[0].Handler = nil

	fresh, ok := app.GetInterface("blocks-1")
	if !ok {
		t.Fatal("blocks interface not found after mutation")
	}
	if fresh.Events[0].Inputs[0] == "mutated" {
		t.Fatalf("GetInterface returned shared event input slice")
	}
	if fresh.Events[0].Handler == nil {
		t.Fatalf("GetInterface returned shared event handler field")
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

func TestPredictEndpointLeavesMediaMapWithoutIDUnchanged(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	app.Interface(
		goleo.Handler(func(input map[string]any) (string, error) {
			name, _ := input["name"].(string)
			return name, nil
		}),
		goleo.Inputs(goleo.Audio("Prompt audio")),
		goleo.Outputs(goleo.Textbox("Result")),
	)

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	resp, err := http.Post(
		server.URL+"/api/predict",
		"application/json",
		strings.NewReader(`{"interface_id":"interface-1","data":[{"name":"raw.wav"}]}`),
	)
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
		t.Fatalf("decode predict response: %v", err)
	}
	if len(got.Data) != 1 || got.Data[0] != "raw.wav" {
		t.Fatalf("data = %#v, want raw.wav", got.Data)
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

func TestEventEndpointInvokesBlocksHandler(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	var prompt, out goleo.Component
	app.Blocks(func(blocks *goleo.Blocks) {
		prompt = blocks.Textbox("Prompt")
		out = blocks.Textbox("Result")
		run := blocks.Button("Run")

		run.Click(
			goleo.Handler(func(input string) (string, error) {
				return strings.ToUpper(input), nil
			}),
			goleo.Inputs(prompt),
			goleo.Outputs(out),
		)
	})

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	requestBody, err := json.Marshal(map[string]any{
		"interface_id": "blocks-1",
		"event_id":     "blocks-1-event-1",
		"data": map[string]any{
			prompt.ID: "hello",
		},
	})
	if err != nil {
		t.Fatalf("marshal event request: %v", err)
	}
	resp, err := http.Post(server.URL+"/api/event", "application/json", bytes.NewReader(requestBody))
	if err != nil {
		t.Fatalf("post event: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var got struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode event response: %v", err)
	}
	if got.Data[out.ID] != "HELLO" {
		t.Fatalf("data = %#v, want output HELLO", got.Data)
	}
}

func TestEventEndpointReturnsUpdateEnvelope(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	var prompt, run goleo.Component
	app.Blocks(func(blocks *goleo.Blocks) {
		prompt = blocks.Textbox("Prompt")
		run = blocks.Button("Run")

		prompt.Change(
			goleo.Handler(func(input string) (goleo.Update, error) {
				return goleo.Disabled(input == ""), nil
			}),
			goleo.Inputs(prompt),
			goleo.Outputs(run),
		)
	})

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	requestBody, err := json.Marshal(map[string]any{
		"interface_id": "blocks-1",
		"event_id":     "blocks-1-event-1",
		"data": map[string]any{
			prompt.ID: "",
		},
	})
	if err != nil {
		t.Fatalf("marshal event request: %v", err)
	}
	resp, err := http.Post(server.URL+"/api/event", "application/json", bytes.NewReader(requestBody))
	if err != nil {
		t.Fatalf("post event: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var got struct {
		Data map[string]map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode event response: %v", err)
	}
	update := got.Data[run.ID]
	if update["kind"] != "update" || update["__goleo_update__"] != true || update["disabled"] != true {
		t.Fatalf("update = %#v, want marked disabled update", update)
	}
	if _, ok := update["visible"]; ok {
		t.Fatalf("update = %#v, did not want unset visible field", update)
	}
}

func TestEventEndpointReturnsKindUpdateMapAsOrdinaryData(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	var out goleo.Component
	app.Blocks(func(blocks *goleo.Blocks) {
		out = blocks.JSON("Result")
		run := blocks.Button("Run")
		run.Click(
			goleo.Handler(func() (map[string]any, error) {
				return map[string]any{"kind": "update", "status": "ok"}, nil
			}),
			goleo.Inputs(),
			goleo.Outputs(out),
		)
	})

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	requestBody, err := json.Marshal(map[string]any{
		"interface_id": "blocks-1",
		"event_id":     "blocks-1-event-1",
		"data":         map[string]any{},
	})
	if err != nil {
		t.Fatalf("marshal event request: %v", err)
	}
	resp, err := http.Post(server.URL+"/api/event", "application/json", bytes.NewReader(requestBody))
	if err != nil {
		t.Fatalf("post event: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var got struct {
		Data map[string]map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode event response: %v", err)
	}
	value := got.Data[out.ID]
	if value["kind"] != "update" || value["status"] != "ok" {
		t.Fatalf("data = %#v, want ordinary kind update map", value)
	}
	if _, ok := value["__goleo_update__"]; ok {
		t.Fatalf("data = %#v, did not want update marker", value)
	}
}

func TestEventEndpointDehydratesUpdateValueMediaOutput(t *testing.T) {
	t.Parallel()

	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "reply.wav")
	if err := os.WriteFile(outputPath, []byte("voice"), 0o600); err != nil {
		t.Fatalf("write output audio: %v", err)
	}

	app := goleo.New()
	var prompt, reply goleo.Component
	app.Blocks(func(blocks *goleo.Blocks) {
		prompt = blocks.Textbox("Prompt")
		reply = blocks.Audio("Reply")
		run := blocks.Button("Run")
		run.Click(
			goleo.Handler(func(input string) (goleo.Update, error) {
				return goleo.Value(goleo.AudioOutput{
					Name:        "reply.wav",
					ContentType: "audio/wav",
					Path:        outputPath,
				}), nil
			}),
			goleo.Inputs(prompt),
			goleo.Outputs(reply),
		)
	})

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	requestBody, err := json.Marshal(map[string]any{
		"interface_id": "blocks-1",
		"event_id":     "blocks-1-event-1",
		"data": map[string]any{
			prompt.ID: "hello",
		},
	})
	if err != nil {
		t.Fatalf("marshal event request: %v", err)
	}
	resp, err := http.Post(server.URL+"/api/event", "application/json", bytes.NewReader(requestBody))
	if err != nil {
		t.Fatalf("post event: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var got struct {
		Data map[string]map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode event response: %v", err)
	}
	update := got.Data[reply.ID]
	if update["kind"] != "update" {
		t.Fatalf("update kind = %#v, want update", update["kind"])
	}
	value, ok := update["value"].(map[string]any)
	if !ok {
		t.Fatalf("update value = %#v, want asset descriptor", update["value"])
	}
	if value["url"] == "" || value["name"] != "reply.wav" {
		t.Fatalf("asset descriptor = %#v, want url and reply.wav", value)
	}
}

func TestEventEndpointUsesOmittedStateInput(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	var prompt, out goleo.Component
	app.Blocks(func(blocks *goleo.Blocks) {
		prompt = blocks.Textbox("Prompt")
		state := blocks.State("Memory", goleo.WithDefault("saved"))
		out = blocks.Textbox("Result")
		run := blocks.Button("Run")
		run.Click(
			goleo.Handler(func(input string, memory string) (string, error) {
				return input + ":" + memory, nil
			}),
			goleo.Inputs(prompt, state),
			goleo.Outputs(out),
		)
	})

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	requestBody, err := json.Marshal(map[string]any{
		"interface_id": "blocks-1",
		"event_id":     "blocks-1-event-1",
		"data": map[string]any{
			prompt.ID: "hello",
		},
	})
	if err != nil {
		t.Fatalf("marshal event request: %v", err)
	}
	resp, err := http.Post(server.URL+"/api/event", "application/json", bytes.NewReader(requestBody))
	if err != nil {
		t.Fatalf("post event: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var got struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode event response: %v", err)
	}
	if got.Data[out.ID] != "hello:saved" {
		t.Fatalf("data = %#v, want state-backed output", got.Data)
	}
}

func TestEventEndpointStoresUpdateValueForStateOutput(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	var prompt, state, out goleo.Component
	app.Blocks(func(blocks *goleo.Blocks) {
		prompt = blocks.Textbox("Prompt")
		state = blocks.State("Memory", goleo.WithDefault("initial"))
		out = blocks.Textbox("Result")
		save := blocks.Button("Save")
		read := blocks.Button("Read")
		save.Click(
			goleo.Handler(func(input string) (goleo.Update, error) {
				return goleo.Value(input), nil
			}),
			goleo.Inputs(prompt),
			goleo.Outputs(state),
		)
		read.Click(
			goleo.Handler(func(memory string) (string, error) {
				return memory, nil
			}),
			goleo.Inputs(state),
			goleo.Outputs(out),
		)
	})

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	saveBody, err := json.Marshal(map[string]any{
		"interface_id": "blocks-1",
		"event_id":     "blocks-1-event-1",
		"data": map[string]any{
			prompt.ID: "next",
		},
	})
	if err != nil {
		t.Fatalf("marshal save request: %v", err)
	}
	saveResp, err := http.Post(server.URL+"/api/event", "application/json", bytes.NewReader(saveBody))
	if err != nil {
		t.Fatalf("post save event: %v", err)
	}
	defer saveResp.Body.Close()

	if saveResp.StatusCode != http.StatusOK {
		t.Fatalf("save status = %d, want %d", saveResp.StatusCode, http.StatusOK)
	}
	var saveGot struct {
		Data map[string]map[string]any `json:"data"`
	}
	if err := json.NewDecoder(saveResp.Body).Decode(&saveGot); err != nil {
		t.Fatalf("decode save response: %v", err)
	}
	if update := saveGot.Data[state.ID]; update["kind"] != "update" || update["value"] != "next" {
		t.Fatalf("state update = %#v, want update value next", update)
	}

	readBody, err := json.Marshal(map[string]any{
		"interface_id": "blocks-1",
		"event_id":     "blocks-1-event-2",
		"data":         map[string]any{},
	})
	if err != nil {
		t.Fatalf("marshal read request: %v", err)
	}
	readResp, err := http.Post(server.URL+"/api/event", "application/json", bytes.NewReader(readBody))
	if err != nil {
		t.Fatalf("post read event: %v", err)
	}
	defer readResp.Body.Close()

	if readResp.StatusCode != http.StatusOK {
		t.Fatalf("read status = %d, want %d", readResp.StatusCode, http.StatusOK)
	}
	var readGot struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(readResp.Body).Decode(&readGot); err != nil {
		t.Fatalf("decode read response: %v", err)
	}
	if readGot.Data[out.ID] != "next" {
		t.Fatalf("data = %#v, want stored update value next", readGot.Data)
	}
}

func TestEventEndpointRejectsUnknownSourceComponentID(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	var prompt goleo.Component
	app.Blocks(func(blocks *goleo.Blocks) {
		prompt = blocks.Textbox("Prompt")
		out := blocks.Textbox("Result")
		blocks.BindEvent(
			"click",
			goleo.Component{ID: "missing-source"},
			goleo.Handler(func(input string) (string, error) { return input, nil }),
			goleo.Inputs(prompt),
			goleo.Outputs(out),
		)
	})

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	requestBody, err := json.Marshal(map[string]any{
		"interface_id": "blocks-1",
		"event_id":     "blocks-1-event-1",
		"data": map[string]any{
			prompt.ID: "hello",
		},
	})
	if err != nil {
		t.Fatalf("marshal event request: %v", err)
	}
	resp, err := http.Post(server.URL+"/api/event", "application/json", bytes.NewReader(requestBody))
	if err != nil {
		t.Fatalf("post event: %v", err)
	}
	defer resp.Body.Close()

	assertEventInternalError(t, resp, `source component "missing-source" not found`)
}

func TestEventEndpointAllowsSourceLessLoadEvent(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	var out goleo.Component
	app.Blocks(func(blocks *goleo.Blocks) {
		out = blocks.Textbox("Result")
		blocks.Load(
			goleo.Handler(func() (string, error) { return "loaded", nil }),
			goleo.Outputs(out),
		)
	})

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	resp, err := http.Post(
		server.URL+"/api/event",
		"application/json",
		strings.NewReader(`{"interface_id":"blocks-1","event_id":"blocks-1-event-1","data":{}}`),
	)
	if err != nil {
		t.Fatalf("post event: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var got struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode event response: %v", err)
	}
	if got.Data[out.ID] != "loaded" {
		t.Fatalf("data = %#v, want loaded", got.Data)
	}
}

func TestEventEndpointQueueLimitReturnsError(t *testing.T) {
	app := goleo.New()
	app.ConfigureQueue(1, 0)
	started := make(chan struct{})
	unblock := make(chan struct{})
	var input goleo.Component
	app.Blocks(func(blocks *goleo.Blocks) {
		input = blocks.Textbox("Prompt")
		output := blocks.Textbox("Result")
		run := blocks.Button("Run")
		run.Click(
			goleo.Handler(func(value string) (string, error) {
				close(started)
				<-unblock
				return value, nil
			}),
			goleo.Inputs(input),
			goleo.Outputs(output),
		)
	})

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	requestBody := func(value string) io.Reader {
		body, err := json.Marshal(map[string]any{
			"interface_id": "blocks-1",
			"event_id":     "blocks-1-event-1",
			"data": map[string]any{
				input.ID: value,
			},
		})
		if err != nil {
			t.Fatalf("marshal event request: %v", err)
		}
		return bytes.NewReader(body)
	}

	firstDone := make(chan struct{})
	var firstErr error
	go func() {
		resp, err := http.Post(server.URL+"/api/event", "application/json", requestBody("first"))
		if err != nil {
			firstErr = err
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		close(firstDone)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first event handler did not start")
	}
	unblocked := false
	defer func() {
		if !unblocked {
			close(unblock)
		}
	}()

	resp, err := http.Post(server.URL+"/api/event", "application/json", requestBody("second"))
	if err != nil {
		t.Fatalf("post second event: %v", err)
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

	close(unblock)
	unblocked = true
	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("first event request did not finish after unblock")
	}
	if firstErr != nil {
		t.Fatalf("first event request failed: %v", firstErr)
	}
}

func TestEventEndpointSupportsCancellation(t *testing.T) {
	app := goleo.New()
	var cancelled int32
	started := make(chan struct{})
	var input goleo.Component
	app.Blocks(func(blocks *goleo.Blocks) {
		input = blocks.Textbox("Prompt")
		output := blocks.Textbox("Result")
		run := blocks.Button("Run")
		run.Click(
			goleo.Handler(func(ctx context.Context, value string) (string, error) {
				close(started)
				<-ctx.Done()
				atomic.StoreInt32(&cancelled, 1)
				return "", ctx.Err()
			}),
			goleo.Inputs(input),
			goleo.Outputs(output),
		)
	})

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	requestBody, err := json.Marshal(map[string]any{
		"interface_id": "blocks-1",
		"event_id":     "blocks-1-event-1",
		"data": map[string]any{
			input.ID: "hello",
		},
	})
	if err != nil {
		t.Fatalf("marshal event request: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/event", bytes.NewReader(requestBody))
	if err != nil {
		t.Fatalf("create event request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "blocks-cancel-test")

	done := make(chan struct{})
	go func() {
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
		}
		close(done)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("event handler did not start")
	}

	cancelResp, err := http.Post(
		server.URL+"/api/cancel",
		"application/json",
		strings.NewReader(`{"request_id":"blocks-cancel-test"}`),
	)
	if err != nil {
		t.Fatalf("post cancel: %v", err)
	}
	defer cancelResp.Body.Close()

	if cancelResp.StatusCode != http.StatusOK {
		t.Fatalf("cancel status = %d, want %d", cancelResp.StatusCode, http.StatusOK)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("event request did not finish after cancellation")
	}

	if atomic.LoadInt32(&cancelled) == 0 {
		t.Fatalf("event handler was not cancelled")
	}
}

func TestEventEndpointSupportsBodyRequestIDCancellation(t *testing.T) {
	app := goleo.New()
	var cancelled int32
	started := make(chan struct{})
	var input goleo.Component
	app.Blocks(func(blocks *goleo.Blocks) {
		input = blocks.Textbox("Prompt")
		output := blocks.Textbox("Result")
		run := blocks.Button("Run")
		run.Click(
			goleo.Handler(func(ctx context.Context, value string) (string, error) {
				close(started)
				<-ctx.Done()
				atomic.StoreInt32(&cancelled, 1)
				return "", ctx.Err()
			}),
			goleo.Inputs(input),
			goleo.Outputs(output),
		)
	})

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	requestBody, err := json.Marshal(map[string]any{
		"request_id":   "blocks-body-cancel-test",
		"interface_id": "blocks-1",
		"event_id":     "blocks-1-event-1",
		"data": map[string]any{
			input.ID: "hello",
		},
	})
	if err != nil {
		t.Fatalf("marshal event request: %v", err)
	}
	requestContext, cancelRequest := context.WithCancel(context.Background())
	defer cancelRequest()
	req, err := http.NewRequestWithContext(requestContext, http.MethodPost, server.URL+"/api/event", bytes.NewReader(requestBody))
	if err != nil {
		t.Fatalf("create event request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	done := make(chan struct{})
	go func() {
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
		}
		close(done)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("event handler did not start")
	}

	cancelResp, err := http.Post(
		server.URL+"/api/cancel",
		"application/json",
		strings.NewReader(`{"request_id":"blocks-body-cancel-test"}`),
	)
	if err != nil {
		t.Fatalf("post cancel: %v", err)
	}
	defer cancelResp.Body.Close()

	if cancelResp.StatusCode != http.StatusOK {
		cancelRequest()
		t.Fatalf("cancel status = %d, want %d", cancelResp.StatusCode, http.StatusOK)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("event request did not finish after cancellation")
	}

	if atomic.LoadInt32(&cancelled) == 0 {
		t.Fatalf("event handler was not cancelled")
	}
}

func TestEventEndpointRejectsMismatchedHeaderAndBodyRequestID(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	var prompt goleo.Component
	app.Blocks(func(blocks *goleo.Blocks) {
		prompt = blocks.Textbox("Prompt")
		out := blocks.Textbox("Result")
		run := blocks.Button("Run")
		run.Click(
			goleo.Handler(func(input string) (string, error) { return input, nil }),
			goleo.Inputs(prompt),
			goleo.Outputs(out),
		)
	})

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	requestBody, err := json.Marshal(map[string]any{
		"request_id":   "body-request-id",
		"interface_id": "blocks-1",
		"event_id":     "blocks-1-event-1",
		"data": map[string]any{
			prompt.ID: "hello",
		},
	})
	if err != nil {
		t.Fatalf("marshal event request: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/event", bytes.NewReader(requestBody))
	if err != nil {
		t.Fatalf("create event request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "header-request-id")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post event: %v", err)
	}
	defer resp.Body.Close()

	assertEventBadRequest(t, resp, "request_id mismatch")
}

func TestEventEndpointUsesBodyRequestIDAsCanonicalHeader(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	var prompt goleo.Component
	app.Blocks(func(blocks *goleo.Blocks) {
		prompt = blocks.Textbox("Prompt")
		out := blocks.Textbox("Result")
		run := blocks.Button("Run")
		run.Click(
			goleo.Handler(func(input string) (string, error) { return input, nil }),
			goleo.Inputs(prompt),
			goleo.Outputs(out),
		)
	})

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	requestBody, err := json.Marshal(map[string]any{
		"request_id":   "body-canonical-id",
		"interface_id": "blocks-1",
		"event_id":     "blocks-1-event-1",
		"data": map[string]any{
			prompt.ID: "hello",
		},
	})
	if err != nil {
		t.Fatalf("marshal event request: %v", err)
	}
	resp, err := http.Post(server.URL+"/api/event", "application/json", bytes.NewReader(requestBody))
	if err != nil {
		t.Fatalf("post event: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if got := resp.Header.Get("X-Request-ID"); got != "body-canonical-id" {
		t.Fatalf("X-Request-ID = %q, want body-canonical-id", got)
	}
}

func TestEventEndpointReturnsClearErrorForInvalidInterface(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	app.Blocks(func(blocks *goleo.Blocks) {
		prompt := blocks.Textbox("Prompt")
		out := blocks.Textbox("Result")
		run := blocks.Button("Run")
		run.Click(
			goleo.Handler(func(input string) (string, error) { return input, nil }),
			goleo.Inputs(prompt),
			goleo.Outputs(out),
		)
	})

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	resp, err := http.Post(
		server.URL+"/api/event",
		"application/json",
		strings.NewReader(`{"interface_id":"missing","event_id":"blocks-1-event-1","data":{}}`),
	)
	if err != nil {
		t.Fatalf("post event: %v", err)
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
		t.Fatalf("decode error response: %v", err)
	}
	if got.Error.Code != "not_found" || !strings.Contains(got.Error.Message, `interface "missing" not found`) {
		t.Fatalf("error = %#v, want clear missing interface", got.Error)
	}
}

func TestEventEndpointReturnsClearErrorForInvalidEventID(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	app.Blocks(func(blocks *goleo.Blocks) {
		prompt := blocks.Textbox("Prompt")
		out := blocks.Textbox("Result")
		run := blocks.Button("Run")
		run.Click(
			goleo.Handler(func(input string) (string, error) { return input, nil }),
			goleo.Inputs(prompt),
			goleo.Outputs(out),
		)
	})

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	resp, err := http.Post(
		server.URL+"/api/event",
		"application/json",
		strings.NewReader(`{"interface_id":"blocks-1","event_id":"missing-event","data":{}}`),
	)
	if err != nil {
		t.Fatalf("post event: %v", err)
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
		t.Fatalf("decode error response: %v", err)
	}
	if got.Error.Code != "not_found" || !strings.Contains(got.Error.Message, `event "missing-event" not found for interface "blocks-1"`) {
		t.Fatalf("error = %#v, want clear missing event", got.Error)
	}
}

func TestEventEndpointRejectsMalformedDataShape(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	app.Blocks(func(blocks *goleo.Blocks) {
		prompt := blocks.Textbox("Prompt")
		out := blocks.Textbox("Result")
		run := blocks.Button("Run")
		run.Click(
			goleo.Handler(func(input string) (string, error) { return input, nil }),
			goleo.Inputs(prompt),
			goleo.Outputs(out),
		)
	})

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	resp, err := http.Post(
		server.URL+"/api/event",
		"application/json",
		strings.NewReader(`{"interface_id":"blocks-1","event_id":"blocks-1-event-1","data":[]}`),
	)
	if err != nil {
		t.Fatalf("post event: %v", err)
	}
	defer resp.Body.Close()

	assertEventBadRequest(t, resp, "cannot unmarshal array")
}

func TestEventEndpointRejectsUnknownDataComponentID(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	var prompt goleo.Component
	app.Blocks(func(blocks *goleo.Blocks) {
		prompt = blocks.Textbox("Prompt")
		out := blocks.Textbox("Result")
		run := blocks.Button("Run")
		run.Click(
			goleo.Handler(func(input string) (string, error) { return input, nil }),
			goleo.Inputs(prompt),
			goleo.Outputs(out),
		)
	})

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	requestBody, err := json.Marshal(map[string]any{
		"interface_id": "blocks-1",
		"event_id":     "blocks-1-event-1",
		"data": map[string]any{
			prompt.ID: "hello",
			"unknown": "ignored",
		},
	})
	if err != nil {
		t.Fatalf("marshal event request: %v", err)
	}
	resp, err := http.Post(server.URL+"/api/event", "application/json", bytes.NewReader(requestBody))
	if err != nil {
		t.Fatalf("post event: %v", err)
	}
	defer resp.Body.Close()

	assertEventBadRequest(t, resp, `unknown input component "unknown"`)
}

func TestEventEndpointRejectsMissingInputKey(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	var prompt goleo.Component
	app.Blocks(func(blocks *goleo.Blocks) {
		prompt = blocks.Textbox("Prompt")
		out := blocks.Textbox("Result")
		run := blocks.Button("Run")
		run.Click(
			goleo.Handler(func(input string) (string, error) { return input, nil }),
			goleo.Inputs(prompt),
			goleo.Outputs(out),
		)
	})

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	resp, err := http.Post(
		server.URL+"/api/event",
		"application/json",
		strings.NewReader(`{"interface_id":"blocks-1","event_id":"blocks-1-event-1","data":{}}`),
	)
	if err != nil {
		t.Fatalf("post event: %v", err)
	}
	defer resp.Body.Close()

	assertEventBadRequest(t, resp, fmt.Sprintf("missing input %q", prompt.ID))
}

func TestEventEndpointAllowsHiddenInputOmittedFromData(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	var hidden, visible, out goleo.Component
	app.Blocks(func(blocks *goleo.Blocks) {
		hidden = blocks.Textbox("Hidden", goleo.WithDefault("hidden-default"))
		visible = blocks.Textbox("Visible")
		out = blocks.Textbox("Result")
		run := blocks.Button("Run")
		run.Click(
			goleo.Handler(func(hiddenInput string, visibleInput string) (string, error) {
				if hiddenInput != "" {
					return "unexpected hidden input: " + hiddenInput, nil
				}
				return visibleInput, nil
			}),
			goleo.Inputs(hidden, visible),
			goleo.Outputs(out),
		)
	})

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	requestBody, err := json.Marshal(map[string]any{
		"interface_id": "blocks-1",
		"event_id":     "blocks-1-event-1",
		"data": map[string]any{
			visible.ID: "shown",
		},
		"hidden": []string{hidden.ID},
	})
	if err != nil {
		t.Fatalf("marshal event request: %v", err)
	}
	resp, err := http.Post(server.URL+"/api/event", "application/json", bytes.NewReader(requestBody))
	if err != nil {
		t.Fatalf("post event: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want %d, body = %s", resp.StatusCode, http.StatusOK, body)
	}

	var got struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode event response: %v", err)
	}
	if got.Data[out.ID] != "shown" {
		t.Fatalf("data = %#v, want visible value", got.Data)
	}
}

func TestEventEndpointRejectsHiddenInputNotPartOfEventInputs(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	var prompt, extra goleo.Component
	app.Blocks(func(blocks *goleo.Blocks) {
		prompt = blocks.Textbox("Prompt")
		extra = blocks.Textbox("Extra")
		out := blocks.Textbox("Result")
		run := blocks.Button("Run")
		run.Click(
			goleo.Handler(func(input string) (string, error) { return input, nil }),
			goleo.Inputs(prompt),
			goleo.Outputs(out),
		)
	})

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	requestBody, err := json.Marshal(map[string]any{
		"interface_id": "blocks-1",
		"event_id":     "blocks-1-event-1",
		"data": map[string]any{
			prompt.ID: "hello",
		},
		"hidden": []string{extra.ID},
	})
	if err != nil {
		t.Fatalf("marshal event request: %v", err)
	}
	resp, err := http.Post(server.URL+"/api/event", "application/json", bytes.NewReader(requestBody))
	if err != nil {
		t.Fatalf("post event: %v", err)
	}
	defer resp.Body.Close()

	assertEventBadRequest(t, resp, fmt.Sprintf("hidden input %q is not part of event inputs", extra.ID))
}

func TestEventEndpointRejectsUnknownHiddenInputID(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	var prompt goleo.Component
	app.Blocks(func(blocks *goleo.Blocks) {
		prompt = blocks.Textbox("Prompt")
		out := blocks.Textbox("Result")
		run := blocks.Button("Run")
		run.Click(
			goleo.Handler(func(input string) (string, error) { return input, nil }),
			goleo.Inputs(prompt),
			goleo.Outputs(out),
		)
	})

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	requestBody, err := json.Marshal(map[string]any{
		"interface_id": "blocks-1",
		"event_id":     "blocks-1-event-1",
		"data": map[string]any{
			prompt.ID: "hello",
		},
		"hidden": []string{"missing-component"},
	})
	if err != nil {
		t.Fatalf("marshal event request: %v", err)
	}
	resp, err := http.Post(server.URL+"/api/event", "application/json", bytes.NewReader(requestBody))
	if err != nil {
		t.Fatalf("post event: %v", err)
	}
	defer resp.Body.Close()

	assertEventBadRequest(t, resp, `unknown hidden component "missing-component"`)
}

func TestEventEndpointRejectsClientProvidedStateInput(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	var prompt, state goleo.Component
	app.Blocks(func(blocks *goleo.Blocks) {
		prompt = blocks.Textbox("Prompt")
		state = blocks.State("Memory", goleo.WithDefault("saved"))
		out := blocks.Textbox("Result")
		run := blocks.Button("Run")
		run.Click(
			goleo.Handler(func(input string, memory string) (string, error) {
				return input + ":" + memory, nil
			}),
			goleo.Inputs(prompt, state),
			goleo.Outputs(out),
		)
	})

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	requestBody, err := json.Marshal(map[string]any{
		"interface_id": "blocks-1",
		"event_id":     "blocks-1-event-1",
		"data": map[string]any{
			prompt.ID: "hello",
			state.ID:  "client",
		},
	})
	if err != nil {
		t.Fatalf("marshal event request: %v", err)
	}
	resp, err := http.Post(server.URL+"/api/event", "application/json", bytes.NewReader(requestBody))
	if err != nil {
		t.Fatalf("post event: %v", err)
	}
	defer resp.Body.Close()

	assertEventBadRequest(t, resp, fmt.Sprintf("state input %q cannot be set by client", state.ID))
}

func TestEventEndpointRejectsMediaInputWithoutID(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	var audio goleo.Component
	app.Blocks(func(blocks *goleo.Blocks) {
		audio = blocks.Audio("Prompt")
		out := blocks.Textbox("Result")
		run := blocks.Button("Run")
		run.Click(
			goleo.Handler(func(input goleo.AudioInput) (string, error) { return input.Name, nil }),
			goleo.Inputs(audio),
			goleo.Outputs(out),
		)
	})

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	requestBody, err := json.Marshal(map[string]any{
		"interface_id": "blocks-1",
		"event_id":     "blocks-1-event-1",
		"data": map[string]any{
			audio.ID: map[string]any{"name": "prompt.wav"},
		},
	})
	if err != nil {
		t.Fatalf("marshal event request: %v", err)
	}
	resp, err := http.Post(server.URL+"/api/event", "application/json", bytes.NewReader(requestBody))
	if err != nil {
		t.Fatalf("post event: %v", err)
	}
	defer resp.Body.Close()

	assertEventBadRequest(t, resp, fmt.Sprintf("media input %q requires asset id", audio.ID))
}

func TestEventEndpointRejectsMediaInputNonObject(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	var audio goleo.Component
	app.Blocks(func(blocks *goleo.Blocks) {
		audio = blocks.Audio("Prompt")
		out := blocks.Textbox("Result")
		run := blocks.Button("Run")
		run.Click(
			goleo.Handler(func(input goleo.AudioInput) (string, error) { return input.Name, nil }),
			goleo.Inputs(audio),
			goleo.Outputs(out),
		)
	})

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	requestBody, err := json.Marshal(map[string]any{
		"interface_id": "blocks-1",
		"event_id":     "blocks-1-event-1",
		"data": map[string]any{
			audio.ID: "raw",
		},
	})
	if err != nil {
		t.Fatalf("marshal event request: %v", err)
	}
	resp, err := http.Post(server.URL+"/api/event", "application/json", bytes.NewReader(requestBody))
	if err != nil {
		t.Fatalf("post event: %v", err)
	}
	defer resp.Body.Close()

	assertEventBadRequest(t, resp, fmt.Sprintf("media input %q must be an object", audio.ID))
}

func TestEventEndpointRejectsMissingAssetID(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	var audio goleo.Component
	app.Blocks(func(blocks *goleo.Blocks) {
		audio = blocks.Audio("Prompt")
		out := blocks.Textbox("Result")
		run := blocks.Button("Run")
		run.Click(
			goleo.Handler(func(input goleo.AudioInput) (string, error) { return input.Name, nil }),
			goleo.Inputs(audio),
			goleo.Outputs(out),
		)
	})

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	requestBody, err := json.Marshal(map[string]any{
		"interface_id": "blocks-1",
		"event_id":     "blocks-1-event-1",
		"data": map[string]any{
			audio.ID: map[string]any{"id": "missing-asset"},
		},
	})
	if err != nil {
		t.Fatalf("marshal event request: %v", err)
	}
	resp, err := http.Post(server.URL+"/api/event", "application/json", bytes.NewReader(requestBody))
	if err != nil {
		t.Fatalf("post event: %v", err)
	}
	defer resp.Body.Close()

	assertEventBadRequest(t, resp, `asset "missing-asset" not found`)
}

func TestEventEndpointRejectsUnavailableUploadedAssetID(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	var audio goleo.Component
	app.Blocks(func(blocks *goleo.Blocks) {
		audio = blocks.Audio("Prompt")
		out := blocks.Textbox("Result")
		run := blocks.Button("Run")
		run.Click(
			goleo.Handler(func(input goleo.AudioInput) (string, error) { return input.Name, nil }),
			goleo.Inputs(audio),
			goleo.Outputs(out),
		)
	})

	firstHandler := app.Handler()
	firstServer := httptest.NewServer(firstHandler)

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

	uploadResp, err := http.Post(firstServer.URL+"/api/upload", writer.FormDataContentType(), &uploadBody)
	if err != nil {
		t.Fatalf("post upload: %v", err)
	}
	defer uploadResp.Body.Close()

	var uploaded map[string]any
	if err := json.NewDecoder(uploadResp.Body).Decode(&uploaded); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	assetID, _ := uploaded["id"].(string)
	if assetID == "" {
		t.Fatalf("uploaded asset id is empty: %#v", uploaded)
	}

	firstServer.Close()
	if closer, ok := firstHandler.(interface{ Close() error }); ok {
		if err := closer.Close(); err != nil {
			t.Fatalf("close first handler: %v", err)
		}
	}

	secondServer := httptest.NewServer(app.Handler())
	defer secondServer.Close()

	requestBody, err := json.Marshal(map[string]any{
		"interface_id": "blocks-1",
		"event_id":     "blocks-1-event-1",
		"data": map[string]any{
			audio.ID: map[string]any{"id": assetID},
		},
	})
	if err != nil {
		t.Fatalf("marshal event request: %v", err)
	}
	resp, err := http.Post(secondServer.URL+"/api/event", "application/json", bytes.NewReader(requestBody))
	if err != nil {
		t.Fatalf("post event: %v", err)
	}
	defer resp.Body.Close()

	assertEventBadRequest(t, resp, fmt.Sprintf("asset %q not found", assetID))
}

func TestEventEndpointRejectsUnknownTargetComponentID(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	var prompt goleo.Component
	app.Blocks(func(blocks *goleo.Blocks) {
		prompt = blocks.Textbox("Prompt")
		run := blocks.Button("Run")
		blocks.BindEvent(
			"click",
			run,
			goleo.Handler(func(input string) (string, error) { return input, nil }),
			goleo.Inputs(prompt),
			goleo.Outputs(goleo.Component{ID: "missing-target"}),
		)
	})

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	requestBody, err := json.Marshal(map[string]any{
		"interface_id": "blocks-1",
		"event_id":     "blocks-1-event-1",
		"data": map[string]any{
			prompt.ID: "hello",
		},
	})
	if err != nil {
		t.Fatalf("marshal event request: %v", err)
	}
	resp, err := http.Post(server.URL+"/api/event", "application/json", bytes.NewReader(requestBody))
	if err != nil {
		t.Fatalf("post event: %v", err)
	}
	defer resp.Body.Close()

	assertEventInternalError(t, resp, `component "missing-target" not found`)
}

func TestEventEndpointReturnsHandlerError(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	var prompt goleo.Component
	app.Blocks(func(blocks *goleo.Blocks) {
		prompt = blocks.Textbox("Prompt")
		out := blocks.Textbox("Result")
		run := blocks.Button("Run")
		run.Click(
			goleo.Handler(func(input int) (string, error) { return "", nil }),
			goleo.Inputs(prompt),
			goleo.Outputs(out),
		)
	})

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	requestBody, err := json.Marshal(map[string]any{
		"interface_id": "blocks-1",
		"event_id":     "blocks-1-event-1",
		"data": map[string]any{
			prompt.ID: "not-a-number",
		},
	})
	if err != nil {
		t.Fatalf("marshal event request: %v", err)
	}
	resp, err := http.Post(server.URL+"/api/event", "application/json", bytes.NewReader(requestBody))
	if err != nil {
		t.Fatalf("post event: %v", err)
	}
	defer resp.Body.Close()

	assertEventBadRequest(t, resp, "input 0")
}

func TestEventEndpointReturnsInvalidHandlerBindingError(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	var prompt goleo.Component
	app.Blocks(func(blocks *goleo.Blocks) {
		prompt = blocks.Textbox("Prompt")
		out := blocks.Textbox("Result")
		run := blocks.Button("Run")
		run.Click(
			goleo.Handler("not a function"),
			goleo.Inputs(prompt),
			goleo.Outputs(out),
		)
	})

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	requestBody, err := json.Marshal(map[string]any{
		"interface_id": "blocks-1",
		"event_id":     "blocks-1-event-1",
		"data": map[string]any{
			prompt.ID: "hello",
		},
	})
	if err != nil {
		t.Fatalf("marshal event request: %v", err)
	}
	resp, err := http.Post(server.URL+"/api/event", "application/json", bytes.NewReader(requestBody))
	if err != nil {
		t.Fatalf("post event: %v", err)
	}
	defer resp.Body.Close()

	assertEventBadRequest(t, resp, "handler must be a function")
}

func TestEventEndpointRejectsOutputCardinalityMismatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		handler *goleo.HandlerBinding
		want    string
	}{
		{
			name: "missing output",
			handler: goleo.Handler(func(input string) error {
				return nil
			}),
			want: "event returned 0 outputs for 1 targets",
		},
		{
			name: "extra output",
			handler: goleo.Handler(func(input string) (string, string, error) {
				return input, input, nil
			}),
			want: "event returned 2 outputs for 1 targets",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := goleo.New()
			var prompt goleo.Component
			app.Blocks(func(blocks *goleo.Blocks) {
				prompt = blocks.Textbox("Prompt")
				out := blocks.Textbox("Result")
				run := blocks.Button("Run")
				run.Click(tt.handler, goleo.Inputs(prompt), goleo.Outputs(out))
			})

			server := httptest.NewServer(app.Handler())
			defer server.Close()

			requestBody, err := json.Marshal(map[string]any{
				"interface_id": "blocks-1",
				"event_id":     "blocks-1-event-1",
				"data": map[string]any{
					prompt.ID: "hello",
				},
			})
			if err != nil {
				t.Fatalf("marshal event request: %v", err)
			}
			resp, err := http.Post(server.URL+"/api/event", "application/json", bytes.NewReader(requestBody))
			if err != nil {
				t.Fatalf("post event: %v", err)
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
				t.Fatalf("decode error response: %v", err)
			}
			if got.Error.Code != "internal_error" || !strings.Contains(got.Error.Message, tt.want) {
				t.Fatalf("error = %#v, want %q", got.Error, tt.want)
			}
		})
	}
}

func assertEventBadRequest(t *testing.T, resp *http.Response, wantMessage string) {
	t.Helper()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	var got struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if got.Error.Code != "bad_request" || !strings.Contains(got.Error.Message, wantMessage) {
		t.Fatalf("error = %#v, want message containing %q", got.Error, wantMessage)
	}
}

func assertEventInternalError(t *testing.T, resp *http.Response, wantMessage string) {
	t.Helper()

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
		t.Fatalf("decode error response: %v", err)
	}
	if got.Error.Code != "internal_error" || !strings.Contains(got.Error.Message, wantMessage) {
		t.Fatalf("error = %#v, want message containing %q", got.Error, wantMessage)
	}
}
