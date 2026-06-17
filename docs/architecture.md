# Goleo Architecture

Goleo keeps the root package as a small public facade and puts implementation
behind focused packages. The goal is to make new components, adapters, runtime
bindings, and server features independent from each other.

## Package Boundaries

- `goleo`: public API facade used by application developers.
- `component`: component schema, component options, built-in component constructors.
- `core`: app model, interface registration, schema generation.
- `media`: handler-facing audio inputs/outputs and browser-safe asset descriptors.
- `runtime`: function binding, reflection conversion, streaming handler abstraction.
- `server`: HTTP routes, embedded frontend assets, uploads, assets, SSE, WebSocket voice sessions.
- `adapter`: external model/process adapters.

## Extension Points

### Components

Use `goleo.CustomComponent` for new frontend component types without changing the
root API:

```go
goleo.CustomComponent("audio", "Audio", goleo.WithProp("source", "upload"))
```

Built-in constructors are wrappers around the same schema model, so future
components can be added in `component` and re-exported from `goleo`. First-class
audio uses the same mechanism:

```go
goleo.Audio("Prompt audio", goleo.WithAccept("audio/*"))
```

`Interface` and `Chat` stay request/response and streaming-oriented. `Voice` is
a separate interface kind for long-lived duplex sessions.

### Runtime

Use `goleo.Handler` for regular functions and `goleo.StreamHandler` for
streaming functions. Use `goleo.VoiceHandler` for duplex sessions. The
`runtime` package owns reflection and conversion, so server routes do not need
to know Go function signatures.

Audio-capable handlers use two explicit domain types:

- `goleo.AudioInput`: temporary server-owned file plus metadata and asset URL.
- `goleo.AudioOutput`: handler result that points to a server-readable file.

When the frontend submits an audio descriptor, `server` hydrates it into
`AudioInput` before invoking the handler. When the handler returns
`AudioOutput`, `server` stores it and dehydrates it into a browser-safe
`AudioAsset`.

### Audio Asset Lifecycle

Audio never exposes raw local paths to the browser. The flow is:

1. Frontend uploads a file or microphone recording to `POST /api/upload`.
2. `server` writes the media into temporary storage and returns an asset
   descriptor with `id`, metadata, and `/api/assets/{id}` URL.
3. `POST /api/predict` and `POST /api/stream` hydrate those descriptors back
   into `goleo.AudioInput` before calling the handler.
4. Audio outputs returned from handlers are copied into the same store and
   exposed again through `/api/assets/{id}`.

The asset store is intentionally process-local and temporary. It is meant for
demo/runtime playback, not for durable storage APIs.

### Voice Sessions

`app.Voice(...)` registers a schema surface with `kind: "voice"`. The frontend
binds that surface to `GET /api/voice/{id}/ws`.

The WebSocket event model in v1 is explicit and transport-oriented.

Client -> server:

- `session.start`
- `input.audio`
- `input.stop`
- `output.interrupt`
- `session.close`

Server -> client:

- `session.ready`
- `output.text`
- `output.audio`
- `output.state`
- `error`
- `session.closed`

`runtime.VoiceSession` exposes these events as a bidirectional stream:

```go
event, err := session.Receive()
err = session.Send(goleo.VoiceEvent{Type: "output.text", Text: "heard you"})
err = session.SendAudio(goleo.AudioOutput{Path: "/tmp/reply.wav", ContentType: "audio/wav"})
```

The runtime does not impose STT/TTS provider semantics. Goleo provides the
session, transport, and media plumbing; handlers decide what to do with the
incoming chunks and outgoing events.

### Adapters

Adapters return `*runtime.HandlerBinding`, which means they can be used anywhere
a native handler can be used:

```go
app.Interface(
	goleo.HTTPAdapter(goleo.HTTPAdapterOptions{URL: "http://localhost:9000/predict"}),
	goleo.Inputs(goleo.Textbox("Prompt")),
	goleo.Outputs(goleo.Textbox("Result")),
)
```

New adapters should live in `adapter` or in external packages that return
`*runtime.HandlerBinding`.
