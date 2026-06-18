# Component Guide

This guide lists all built-in components and how to use them in `Inputs(...)` and
`Outputs(...)`.

## Component model

Every component in `goleo` is emitted as schema JSON for frontend rendering:

- `type`: component name.
- `label`: user-facing text.
- `props`: optional metadata and UI options.
- `choices`: selection options for dropdown-like components.

Useful factory functions are exported at package level:

`Textbox`, `Number`, `Slider`, `Checkbox`, `Dropdown`, `Button`, `Markdown`,
`JSON`, `Image`, `Audio`, `File`, `State`, `Row`, `Column`, `Group`, `Chatbot`,
`CustomComponent`.

Use `Inputs(...)` and `Outputs(...)` to pass ordered lists into an `Interface`.

## Shared options

- `WithProp(key, value)` for arbitrary renderer props.
- `WithChoices(values...)` for `Dropdown`.
- `WithPlaceholder(value)` for text-like fields.
- `WithDefault(value)` for initial value.
- `WithMin`, `WithMax`, `WithStep` for numeric controls.
- `WithRows(value)` to set multiline rows.
- `WithDisabled(value)` and `WithVisible(value)`.
- `WithAccept(value)` for mime/type filters on file-like inputs.
- `WithMultiple(value)` for file-like multi-select.
- `WithVisible(value)` to hide components (commonly used for `State`).

## Built-ins

### Textbox

Input/output component for plain text.

- Typical use: prompts, summaries, labels.
- Input-oriented options:
  - `WithPlaceholder`
  - `WithDefault`
  - `WithRows`
- Works as both input and output component.

### Number

Numeric input for integers and decimals.

- Typical use: parameters like temperature, seed, confidence.
- Supports `WithDefault`, `WithMin`, `WithMax`, `WithStep`.

### Slider

Continuous/step-based range input.

- Typical use: tunable ranges in bounded space.
- Supports `WithMin`, `WithMax`, `WithStep`, `WithDefault`.

### Checkbox

Boolean input.

- Typical use: flags, toggles, enable/disable behavior.
- Supports `WithDefault`.

### Dropdown

Single-choice selector.

- Pass choices as second variadic list: `goleo.Dropdown("Label", "a", "b", "c")`.
- Use `WithDefault` for default selected item.

### Button

Action-like control.

- Backend contract is currently renderer-driven; the server-side contract is
  defined by frontend behavior and may be used for form interactions.

### Markdown

Read-only text rendering.

- Use for instructions, status headings, section separators.
- Works well as output component.

### JSON

Structured object rendering.

- Typical use: model metadata, nested map output, diagnostics payload.

### Image

Image output/input component.

- Use as image output for generated image URLs or paths passed through output
  handling.
- File-like options can be used where supported by front-end implementation.

### Audio

First-class media component for input and output.

- Input behavior:
  - file upload and microphone capture map into one value contract.
  - handler-side value is `goleo.AudioInput`.
  - set `WithAccept("audio/*")` or a stricter value to filter browser file picker.
- Output behavior:
  - return `goleo.AudioOutput` from handler.
  - runtime returns `AudioAsset` descriptor with `/api/assets/{id}` URL.
- Typical use:
  - transcription inputs,
  - generated replies,
  - preview playback in forms.

### File

Generic file upload input.

- Handler receives path-backed input descriptors and can read payload through the
  local `Path` field.
- Supports `WithAccept`, `WithMultiple`.
- Common use: PDFs, CSV, images, archives.

### File & Image Outputs

`Audio`, `File`, and `Image` output components use the same output contract:

- handler returns `goleo.AudioOutput`, `goleo.FileOutput`, or `goleo.ImageOutput`;
- runtime stores the payload path into `/api/assets/{id}` and returns descriptor metadata.

### Chatbot

Output list for chat transcript in `Chat`.

- `Chat` creates this implicitly with label `Chat`.
- For custom chat surfaces, pass manually in interface definitions where needed.

### CustomComponent

`CustomComponent(kind, label, options...)` lets you register a non-standard type.

- Works for front-end extensions and experimental integrations.
- Use the same `kind` string expected by your frontend implementation.

### State

`State` is a hidden component for per-interface state persistence.

- placed in `Inputs`, it participates in request argument position and stores value
  in memory between calls;
- placed in `Outputs`, it stores value returned from handler for next render cycle.
- `State` is invisible by default in the form (`visible=false`).

### Layout helpers

`Row`, `Column`, and `Group` are container components used to group and arrange
inputs/outputs.

- `Row` arranges items in horizontal grid columns.
- `Column` stacks items vertically.
- `Group` adds a labeled group.

## Inputs vs outputs

`goleo.Interface` accepts both input and output arrays explicitly:

- use outputs for fields you expect from the handler.
- inputs control what data frontend sends to handler.

`Chat` and `Voice` are opinionated surfaces with fixed behavior:

- `Chat`: only one textbox input and one chatbot output are used.
- `Voice`: no static input/output components; events are sent through websocket
  event loop.

## Value shapes and mapping

### Plain values

Standard Go types convert through JSON encoding/decoding:

- `string`, `bool`, `int`, `float64`, structs, maps, slices.
- custom struct types are supported through standard JSON unmarshal into typed args.

### Audio / File / Image value shape

`Audio`/`File`/`Image` input handler signatures should use `goleo.AudioInput`,
`goleo.FileInput`, or `goleo.ImageInput`.

- `ID`, `Name`, `Size`, `ContentType`, `Path`, `URL`.
- `Path` is server local and exists while the asset is in store.

Return values can be:

- `goleo.AudioOutput` for audio outputs,
- `goleo.FileOutput` for file outputs,
- `goleo.ImageOutput` for image outputs.

### Recommended mapping pattern

- one return value per output component index;
- when output is `Audio`, return `goleo.AudioOutput` and optional `error`.

## Best practices

- Keep output count aligned with the output component count.
- Keep labels short and specific.
- Prefer semantic labels over technical field names.
- Add `WithDisabled(true)` while an interface is intentionally readonly.
- For media components, avoid relying on local filesystem paths on client side; always
  consume returned asset URLs.

## Related docs

- [Usage guide](usage.md)
- [Architecture](architecture.md)
