# Usage Guide

This guide covers how to build apps with `goleo`, how handler signatures map to
UI data, and how file/audio values move through the request pipeline.

## Installation and First Run

```sh
go get github.com/sneiko/goleo
```

Minimal example:

```go
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/sneiko/goleo"
)

func main() {
	app := goleo.New(goleo.WithLogger(slog.New(slog.NewJSONHandler(os.Stdout, nil))))

	app.Interface(
		goleo.Handler(func(input string) (string, error) {
			return "Hello " + input, nil
		}),
		goleo.Inputs(goleo.Textbox("Prompt")),
		goleo.Outputs(goleo.Textbox("Result")),
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := app.LaunchContext(ctx, goleo.LaunchOptions{Addr: ":7871"}); err != nil {
		os.Exit(1)
	}
}
```

Run an example:

```sh
go run ./examples/simple
```

## App lifecycle

Create an app with `goleo.New`, register one or more interfaces, then run it.

- `app.New(options...)` creates an empty app.
- `app.Interface(handler, inputs, outputs)` registers a request/response surface.
- `app.Chat(handler)` registers a fixed chat form.
- `app.Voice(handler)` registers a WebSocket duplex voice surface.
- `app.ConfigureQueue(maxConcurrency, maxQueue)` sets stream/predict concurrency and queue size.
- `goleo.State(label)` adds a hidden state carrier.
- `goleo.Row`, `goleo.Column`, `goleo.Group` build layout groups.

Launch options:

- `LaunchOptions.Addr` for bind address (default `:7860`).
- `ReadTimeout`, `ReadHeaderTimeout`, `WriteTimeout`, `IdleTimeout`.
- `ShutdownTimeout` for graceful shutdown in `LaunchContext`.

Run modes:

- `app.Launch(options)` blocks until `ListenAndServe` exits.
- `app.LaunchContext(ctx, options)` cancels server on context done and calls
  `Shutdown`.
- `app.Server(options)` returns a preconfigured `*http.Server`.
- `app.Handler()` returns the full handler if you want custom process control.

## Interface surface (`Interface`)

`Interface` takes ordered input components and ordered output components.

Request data is serialized as:

```json
{
  "interface_id": "interface-1",
  "data": [ ... ]
}
```

`data` array values are positional. The first component in `Inputs` maps to the
first argument in your handler, and so on.

`goleo.Handler` accepts these handler patterns:

- `func(arg1 T1, arg2 T2) (out1, out2, error)`
- `func(ctx context.Context, arg1 T1) (out1, err)`
- `func(ctx context.Context, arg1 T1, arg2 T2, ... ) (...)`

Rules:

- Output position maps to output order (first output component receives first returned value).
- `error` can be returned as one of return values; if non-nil, request fails.
- If there are no explicit outputs required, return a single `nil` error.

```go
app.Interface(
	goleo.Handler(func(prompt string, age int) (string, error) {
		return fmt.Sprintf("%s is %d years", prompt, age), nil
	}),
	goleo.Inputs(goleo.Textbox("Prompt"), goleo.Number("Age")),
	goleo.Outputs(goleo.Textbox("Reply")),
)
```

## Streaming surface (`Chat`)

Use `Chat` for SSE-style text streaming responses.

`goleo.StreamHandler` accepts:

- `func(arg, emit) error`
- `func(ctx context.Context, arg, emit) error`

`arg` is one input value because `Chat` uses a fixed `Textbox("Message")` input.
Emit chunks by calling the provided `emit` function.

```go
app.Chat(goleo.StreamHandler(func(msg string, emit goleo.EmitFunc) error {
	emit("received: " + msg)
	return nil
}))
```

### Stream status and cancellation

- Status events are emitted for stream handlers: `queued`, `running`, `done`, and `error`.
- Stream requests can be canceled through `POST /api/cancel` by passing `request_id`.
- `request_id` is returned as:
  - `X-Request-ID` response header;
  - `request_id` field in stream event payload.

## Voice surface (`Voice`)

`app.Voice` creates a full-duplex session over WebSocket:

`GET /api/voice/{interface-id}/ws`

`goleo.VoiceHandler` accepts:

- `func(*goleo.VoiceSession) error`
- `func(context.Context, *goleo.VoiceSession) error`

Session API:

- `session.Receive()` returns the next `goleo.VoiceEvent`.
- `session.Send(goleo.VoiceEvent)` sends a control/text/state event.
- `session.SendAudio(goleo.AudioOutput)` sends generated audio as stream output.

Event types:

- Browser -> server: `session.start`, `input.audio`, `input.stop`, `output.interrupt`, `session.close`.
- Server -> browser: `session.ready`, `output.text`, `output.audio`, `output.state`, `error`, `session.closed`.

`input.audio` payload uses `goleo.VoiceAudio` with `mime_type`, `sequence`,
and `data`. `data` is binary audio chunk bytes encoded in JSON transport.

```go
app.Voice(goleo.VoiceHandler(func(session *goleo.VoiceSession) error {
	for {
		event, err := session.Receive()
		if err != nil {
			return err
		}
		switch event.Type {
		case "session.start":
			return session.Send(goleo.VoiceEvent{Type: "session.ready"})
		}
	}
}))
```

## Component value contracts

### JSON and type conversion

For non-media values, the runtime:

- reads input array values from `/api/predict` and `/api/stream`;
- unmarshals each value into target handler argument types.

### File, image and audio inputs

For media inputs (`File`, `Image`, `Audio`):

- browser sends a reference object with `id`, `name`, `size`, `content_type`, `url`;
- the server resolves `id` from its ephemeral asset store;
handler receives dedicated type for each media kind:

```go
type AudioInput struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
	Path string `json:"path"` // local temp path on server
	URL  string `json:"url"`
}
```

`Path` is guaranteed to be server-local for handler-side reading.

`ImageInput` is alias-compatible with `FileInput` in shape, while the output types
`AudioOutput`, `FileOutput`, and `ImageOutput` share the same required fields.

### Media outputs

If an output component has type `audio`, `file`, or `image`, return one of:

```go
type AudioOutput struct {
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	Path        string `json:"path"` // source file path
}
```

The server stores that path in the ephemeral asset store and returns browser-safe descriptors for those output types:

```go
type AudioAsset struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
	URL         string `json:"url"` // always /api/assets/{id}
}
```

## HTTP API surface

The embedded server exposes:

- `GET /api/schema` returns registered interfaces and components.
- `POST /api/predict` handles `Interface` and adapter calls.
- `POST /api/stream` handles `StreamHandler` output.
- `POST /api/upload` accepts `multipart/form-data` with field `file`.
- `POST /api/cancel` accepts `{"request_id":"... "}` to cancel stream work.
- `GET /api/assets/{id}` serves stored media assets.
- `GET /api/voice/{id}/ws` starts duplex voice sessions.

## Audio and assets

- Upload and generated files are stored in ephemeral temp storage.
- Assets are addressed only by generated IDs, never by local filesystem paths.
- Asset descriptors are safe for browser consumption and expire after inactivity
  timeout configured by the server (`30m` in current runtime).

## Adapters

All adapters are callables returning a normal `*goleo.HandlerBinding`.

- `goleo.HTTPAdapter(options)` for generic JSON `/predict`-style services.
- `goleo.OpenAICompatibleAdapter(options)` for `/v1/chat/completions`.
- `goleo.OllamaAdapter(options)` and `goleo.OllamaStreamAdapter(options)`.
- `goleo.ProcessAdapter(command, args...)` for local command piping.

Any adapter can replace a `Handler(...)` in the same `app.Interface(...)` call.

## Examples to inspect

- `examples/simple` — one-shot form.
- `examples/showcase-form` — mixed components.
- `examples/audio` — upload/mic audio with output playback.
- `examples/voice` — websocket duplex voice.
- `examples/chat` — streaming chatbot.
- `examples/showcase-adapters` — external API wrappers.

## Error handling

If your handler panics, the framework wraps it into an internal error and marks
the request as failed.

Errors from handlers are returned as structured JSON with:

```json
{"error":{"code":"bad_request|not_found|internal_error","message":"..."}}
```
