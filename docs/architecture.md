# Architecture Overview

`goleo` is split into small packages with one responsibility:

- `component`: typed UI component constructors and options.
- `core`: app model, interface registration, and schema generation.
- `runtime`: handler binding and callable reflection.
- `media`: shared media payload types (`AudioInput`, `AudioOutput`, `ImageInput`,
  `ImageOutput`, `FileInput`, `FileOutput`) and browser-safe asset descriptors.
- `adapter`: wrappers for external services and process execution.
- `server`: HTTP routes, queueing, cancelation, and asset lifecycle.

## Core flow

For `Interface` and `Chat`:

1. Frontend reads schema from `GET /api/schema`.
2. User interaction produces component values.
3. Frontend posts to `/api/predict` or `/api/stream`.
4. Server hydrates incoming media descriptors to handler types and merges `state`.
5. Runtime builds handler arguments and invokes `Handler` / `StreamHandler`.
6. For media outputs (`audio`, `file`, `image`), runtime stores generated assets in the
   server asset store and returns browser-safe descriptors (`id`, `name`, `size`, `content_type`, `url`).
7. Handler state updates (`State` outputs) are written back to app-scoped memory store.
8. Frontend renders outputs from returned values.

For `Voice`:

1. Browser connects to `/api/voice/{id}/ws`.
2. Server allocates a `VoiceSession`.
3. Client sends `VoiceEvent` JSON messages (`session.start`, `input.audio`, `input.stop`, ...).
4. Server handler reads events through `session.Receive()` and emits via
   `session.Send(...)` and `session.SendAudio(...)`.
5. Server writes each outbound event to WebSocket and converts `output.audio` to asset descriptors.

## Request lifecycle and queues

`/api/predict` and `/api/stream` use an in-memory per-interface queue:

- `app.ConfigureQueue(maxConcurrency, maxQueue)` controls parallelism and wait queue.
- If capacity is exceeded, requests are rejected with HTTP `429` + `code: queue_full`.
- Stream requests can enter queue state and emit `status: queued`.
- Every request gets a `request_id` in:
  - response header `X-Request-ID`;
  - stream payload field `request_id`.
- `/api/cancel` accepts `request_id` and calls request context `cancel`.

## API boundaries

- `goleo` package is a public facade.
- `core` stores application state and interface metadata only.
- `runtime` owns invocation semantics and panic handling.
- `server` owns protocol adaptation, streaming protocol status envelope, queue state, and cleanup.

## Asset model

Assets are stored in process-local temporary directories:

- upload and generated file-like outputs are stored with generated IDs;
- browser receives only URL-based descriptors through `/api/assets/{id}`;
- raw filesystem paths are never sent to browser;
- store entries are evicted on TTL and server shutdown.

Media contracts:

- handler input structs (`AudioInput`, `FileInput`, `ImageInput`) include `path` for local reads.
- handler output structs (`AudioOutput`, `FileOutput`, `ImageOutput`) include `path` for storage;
- frontend gets app-served asset descriptors only (`id`, `url`, metadata).
