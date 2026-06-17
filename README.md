# Goleo

Goleo is a Go-first framework for quick AI demos. It lets you bind Go functions,
HTTP endpoints, OpenAI-compatible APIs, or local processes to a small embedded web UI.

```go
package main

import (
	"log/slog"
	"os"

	"github.com/sneiko/goleo"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	app := goleo.New(goleo.WithLogger(logger))
	app.Interface(
		goleo.Handler(func(input string) (string, error) {
			return "Hello " + input, nil
		}),
		goleo.Inputs(goleo.Textbox("Prompt")),
		goleo.Outputs(goleo.Textbox("Result")),
	)

	if err := app.Launch(goleo.LaunchOptions{Addr: ":7860"}); err != nil {
		logger.Error("goleo server stopped", "error", err)
		os.Exit(1)
	}
}
```

Run the simple example:

```sh
go run ./examples/simple
```

Run a local Ollama streaming chat demo:

```sh
OLLAMA_MODEL=llama3.2 go run ./examples/ollama
```

Run a generic OpenAI-compatible streaming chat demo:

```sh
OPENAI_BASE_URL=http://localhost:11434/v1 OPENAI_MODEL=llama3.2 go run ./examples/openai-stream
```

Configure HTTP server timeouts when needed:

```go
app.Launch(goleo.LaunchOptions{
    Addr:              ":7860",
    ReadHeaderTimeout: 5 * time.Second,
    ReadTimeout:       30 * time.Second,
    WriteTimeout:      0, // keep unset for long-lived streaming responses
    IdleTimeout:       60 * time.Second,
    ShutdownTimeout:   5 * time.Second,
})
```

For graceful shutdown, cancel the context passed to `LaunchContext`:

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()

err := app.LaunchContext(ctx, goleo.LaunchOptions{Addr: ":7860"})
```

For advanced lifecycle control, build the server yourself:

```go
srv := app.Server(goleo.LaunchOptions{Addr: ":7860"})
err := srv.ListenAndServe()
```

Then open `http://localhost:7860`.

## Frontend development

Goleo is still Go-first: the built frontend is committed under
`server/assets`, so users can run examples without installing Node.js.

The embedded UI is developed as a React/Vite/shadcn frontend in `frontend`:

```sh
pnpm --dir frontend install
pnpm --dir frontend dev
pnpm --dir frontend test
pnpm --dir frontend build
```

`pnpm --dir frontend build` writes the static assets consumed by Go's
`go:embed`. Run it after changing frontend code.

Component constructors accept typed options for common UI props:

```go
goleo.Textbox("Prompt", goleo.WithPlaceholder("Ask something..."), goleo.WithDefault("Hello"))
goleo.Slider("Temperature", goleo.WithMin(0), goleo.WithMax(2), goleo.WithStep(0.1), goleo.WithDefault(0.7))
goleo.File("Image", goleo.WithAccept("image/*"))
```

## Logging

Goleo uses `log/slog` for structured logs. Logging is opt-in: if you do not
pass a logger, Goleo stays quiet.

```go
logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
app := goleo.New(goleo.WithLogger(logger))
```

The built-in server logs request completion events with method, path, status,
duration, and `request_id`. If a request includes `X-Request-ID`, Goleo keeps it;
otherwise it generates one and returns it in the response header.

API errors use a structured JSON shape:

```json
{
  "error": {
    "code": "bad_request",
    "message": "interface_id is required"
  }
}
```

## Status

This is an MVP implementation focused on Gradio-like demos:

- `Interface` for function-backed forms
- `Chat` for streaming chat demos
- Embedded frontend assets served by the Go binary
- JSON prediction endpoint, SSE streaming endpoint, and file upload endpoint
- Native Go, HTTP, OpenAI-compatible, Ollama, streaming, and process adapters
- Optional structured logging with request IDs

Production platform features such as auth, persistent storage, hosting, queues,
and multi-user state are intentionally out of scope for v1.

## Architecture

The root `goleo` package is a facade over focused internal packages:

- `component`: component schema and constructors
- `core`: app model and schema generation
- `runtime`: handler binding and streaming abstraction
- `server`: HTTP routes, uploads, SSE, embedded frontend
- `adapter`: HTTP, OpenAI-compatible, and process adapters

See [docs/architecture.md](docs/architecture.md) for extension points.
