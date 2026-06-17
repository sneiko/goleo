# Goleo Architecture

Goleo keeps the root package as a small public facade and puts implementation
behind focused packages. The goal is to make new components, adapters, runtime
bindings, and server features independent from each other.

## Package Boundaries

- `goleo`: public API facade used by application developers.
- `component`: component schema, component options, built-in component constructors.
- `core`: app model, interface registration, schema generation.
- `runtime`: function binding, reflection conversion, streaming handler abstraction.
- `server`: HTTP routes, embedded frontend assets, uploads, SSE.
- `adapter`: external model/process adapters.

## Extension Points

### Components

Use `goleo.CustomComponent` for new frontend component types without changing the
root API:

```go
goleo.CustomComponent("audio", "Audio", goleo.WithProp("source", "upload"))
```

Built-in constructors are wrappers around the same schema model, so future
components can be added in `component` and re-exported from `goleo`.

### Runtime

Use `goleo.Handler` for regular functions and `goleo.StreamHandler` for
streaming functions. The `runtime` package owns reflection and conversion, so
server routes do not need to know Go function signatures.

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
