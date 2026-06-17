# Architecture Overview

`goleo` is split into small packages with one responsibility:

- `component`: typed UI component constructors and options.
- `core`: app model, interface registration, and schema generation.
- `runtime`: handler binding and callable reflection.
- `media`: audio-specific types (`AudioInput`, `AudioOutput`, `AudioAsset`).
- `adapter`: helper wrappers for external services and process execution.
- `server`: HTTP routes, asset lifecycle, schema publishing, streaming, websocket voice.

## Core flow

For `Interface` and `Chat`:

1. Frontend reads schema from `GET /api/schema`.
2. User interaction produces component values.
3. Frontend posts to `/api/predict` or `/api/stream`.
4. Server hydrates asset descriptors to handler types.
5. Runtime builds handler arguments and invokes `Handler`/`StreamHandler`.
6. For output components, `audio` outputs are dehydrated into asset descriptors.
7. Frontend renders outputs from returned values.

For `Voice`:

1. Browser connects to `/api/voice/{id}/ws`.
2. Server launches a `VoiceSession`.
3. Client sends `VoiceEvent` JSON messages.
4. Server handler reads events through `session.Receive()` and emits via
   `session.Send(...)` and `session.SendAudio(...)`.
5. Server encodes each outbound event to WebSocket and writes audio assets as
   `/api/assets/{id}` descriptors.

## API boundaries

- `goleo` package is a public facade over runtime and adapter packages.
- `core` stores application and interface metadata only.
- `server` owns assets and request/response translation.
- `runtime` owns invocation semantics and panic handling.

## Asset model

Assets are stored in process-local temporary directories:

- upload and generated audio are stored with generated IDs;
- browser never receives raw local file paths;
- assets are served through `/api/assets/{id}`;
- the in-memory store tracks last access and evicts expired entries.
