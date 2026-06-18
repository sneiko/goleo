# Blocks/Event API MVP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Добавить в Goleo первый Gradio-like `Blocks` слой: component refs, event bindings, `/api/event`, frontend event dispatcher, `Update` envelope, пример и документацию.

**Architecture:** `Interface`, `Chat` и `Voice` остаются без изменения wire contract. `Blocks` добавляется как новый `Interface.Kind == "blocks"` с отдельными `components` и `events` в schema, а выполнение event handler переиспользует текущие queue/cancel/media/state функции сервера. Frontend получает отдельный renderer `BlocksInterface`, который хранит values по component id и отправляет `click/change/load` события через `/api/event`.

**Tech Stack:** Go 1.24+, `net/http`, existing `runtime.HandlerBinding`, React + TypeScript + Vitest, existing embedded frontend.

---

## Файловая структура

- Create: `runtime/update.go`  
  Хранит `Update`, envelope-константу и helper constructors.
- Create: `core/blocks.go`  
  Хранит `Blocks`, `EventBinding`, регистрацию `Load` и event binder для компонентов.
- Modify: `component/component.go`  
  Добавляет event binder hooks и методы `Click`/`Change` на `Component`, сохраняя `Inputs(...Component)`/`Outputs(...Component)`.
- Modify: `component_facade.go`  
  Не меняет сигнатуры `Inputs`/`Outputs`; они остаются `...Component`.
- Create: `update_facade.go`  
  Экспортирует `Update` и helper constructors.
- Create: `blocks_facade.go`  
  Экспортирует `Blocks` и `ComponentRef` после появления core builder.
- Modify: `core/app.go`  
  Добавляет fields `Components`, `Events`, метод `Blocks`, поиск event binding.
- Modify: `server/server.go`  
  Добавляет `/api/event`, request/response типы, выполнение event handler, update envelope, queue/cancel.
- Modify: `frontend/src/types.ts`  
  Добавляет `EventSchema` и `ComponentUpdate`.
- Modify: `frontend/src/lib/api.ts`  
  Добавляет `sendEvent`.
- Modify: `frontend/src/App.tsx`  
  Добавляет `BlocksInterface`, поддержку `button`, event dispatch и применение updates.
- Modify: `frontend/src/App.test.tsx`, `frontend/src/lib/api.test.ts`  
  Покрывает rendering/click/change/load/update и API client.
- Create: `examples/blocks/main.go`  
  Демонстрирует click/change/load.
- Modify: `README.md`, `docs/usage.md`, `docs/components.md`, `docs/architecture.md`  
  Документирует `Blocks` и `/api/event`.

## Task 1: Runtime Update helpers

**Files:**
- Create: `runtime/update.go`
- Create: `update_facade.go`
- Test: `app_test.go`

- [ ] **Step 1: Write failing tests for Update helpers**

Add to `app_test.go`:

```go
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
```

Also add `reflect` to imports in `app_test.go`.

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
GOCACHE=/tmp/goleo-gocache go test ./... -run TestUpdateHelpersBuildPointerFields
```

Expected: compile failure because `goleo.Disabled`, `goleo.Visible`, `goleo.Label`, and `goleo.Choices` are not defined.

- [ ] **Step 3: Implement `runtime.Update`**

Create `runtime/update.go`:

```go
package runtime

const UpdateKind = "update"

type Update struct {
	Value    any
	Visible  *bool
	Disabled *bool
	Choices  []string
	Label    *string
}

func Value(value any) Update {
	return Update{Value: value}
}

func Visible(value bool) Update {
	return Update{Visible: &value}
}

func Disabled(value bool) Update {
	return Update{Disabled: &value}
}

func Choices(values ...string) Update {
	return Update{Choices: append([]string{}, values...)}
}

func Label(value string) Update {
	return Update{Label: &value}
}
```

- [ ] **Step 4: Preserve existing component list API**

Verify `component/component.go` remains:

```go
func Inputs(components ...Component) List {
	return List{Components: append([]Component{}, components...)}
}

func Outputs(components ...Component) List {
	return List{Components: append([]Component{}, components...)}
}
```

Verify `component_facade.go` remains:

```go
func Inputs(components ...Component) ComponentList {
	return component.Inputs(components...)
}

func Outputs(components ...Component) ComponentList {
	return component.Outputs(components...)
}
```

- [ ] **Step 5: Update public facade**

Create `update_facade.go`:

```go
package goleo

import "github.com/sneiko/goleo/runtime"

type Update = runtime.Update

func Value(value any) Update {
	return runtime.Value(value)
}

func Visible(value bool) Update {
	return runtime.Visible(value)
}

func Disabled(value bool) Update {
	return runtime.Disabled(value)
}

func Choices(values ...string) Update {
	return runtime.Choices(values...)
}

func Label(value string) Update {
	return runtime.Label(value)
}
```

- [ ] **Step 6: Run focused tests**

Run:

```bash
GOCACHE=/tmp/goleo-gocache go test ./... -run TestUpdateHelpersBuildPointerFields
```

Expected: PASS.

- [ ] **Step 7: Commit**

Run:

```bash
git add runtime/update.go update_facade.go app_test.go
git commit -m "feat: add update helpers"
```

## Task 2: Core Blocks builder and schema

**Files:**
- Create: `core/blocks.go`
- Create: `blocks_facade.go`
- Modify: `core/app.go`
- Modify: `component/component.go`
- Test: `app_test.go`

- [ ] **Step 1: Write failing schema test**

Add to `app_test.go`:

```go
func TestBlocksSchemaIncludesComponentsAndEvents(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	app.Blocks(func(blocks *goleo.Blocks) {
		prompt := blocks.Textbox("Prompt")
		run := blocks.Button("Run")
		out := blocks.Textbox("Result")

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
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
GOCACHE=/tmp/goleo-gocache go test ./... -run TestBlocksSchemaIncludesComponentsAndEvents
```

Expected: compile failure because `App.Blocks`, `Interface.Components`, and `Interface.Events` are missing.

- [ ] **Step 3: Add event methods to `component.Component`**

Modify `component/component.go` imports:

```go
import (
	"strings"

	"github.com/sneiko/goleo/runtime"
)
```

Add below `Component`:

```go
type EventBinder interface {
	BindEvent(trigger string, source Component, handler *runtime.HandlerBinding, inputs List, outputs List)
}
```

Add unexported field to `Component`:

```go
eventBinder EventBinder
```

Add helpers:

```go
func WithEventBinder(component Component, binder EventBinder) Component {
	component.eventBinder = binder
	return component
}

func (component Component) Click(handler *runtime.HandlerBinding, inputs List, outputs List) {
	if component.eventBinder == nil {
		return
	}
	component.eventBinder.BindEvent("click", component, handler, inputs, outputs)
}

func (component Component) Change(handler *runtime.HandlerBinding, inputs List, outputs List) {
	if component.eventBinder == nil {
		return
	}
	component.eventBinder.BindEvent("change", component, handler, inputs, outputs)
}
```

`Inputs` and `Outputs` must remain:

```go
func Inputs(components ...Component) List
func Outputs(components ...Component) List
```

- [ ] **Step 4: Create `core/blocks.go`**

Create `core/blocks.go`:

```go
package core

import (
	"strconv"

	"github.com/sneiko/goleo/component"
	"github.com/sneiko/goleo/runtime"
)

type EventBinding struct {
	ID      string   `json:"id"`
	Trigger string   `json:"trigger"`
	Source  string   `json:"source"`
	Inputs  []string `json:"inputs"`
	Outputs []string `json:"outputs"`

	Handler *runtime.HandlerBinding `json:"-"`
}

type Blocks struct {
	id         string
	components []component.Component
	events     []EventBinding
}

func newBlocks(id string) *Blocks {
	return &Blocks{id: id}
}

func (blocks *Blocks) Textbox(label string, options ...component.Option) component.Component {
	return blocks.add(component.Textbox(label, options...))
}

func (blocks *Blocks) Number(label string, options ...component.Option) component.Component {
	return blocks.add(component.Number(label, options...))
}

func (blocks *Blocks) Slider(label string, options ...component.Option) component.Component {
	return blocks.add(component.Slider(label, options...))
}

func (blocks *Blocks) Checkbox(label string, options ...component.Option) component.Component {
	return blocks.add(component.Checkbox(label, options...))
}

func (blocks *Blocks) Dropdown(label string, choices ...string) component.Component {
	return blocks.add(component.Dropdown(label, choices...))
}

func (blocks *Blocks) Button(label string, options ...component.Option) component.Component {
	return blocks.add(component.Button(label, options...))
}

func (blocks *Blocks) Markdown(label string, options ...component.Option) component.Component {
	return blocks.add(component.Markdown(label, options...))
}

func (blocks *Blocks) JSON(label string, options ...component.Option) component.Component {
	return blocks.add(component.JSON(label, options...))
}

func (blocks *Blocks) Image(label string, options ...component.Option) component.Component {
	return blocks.add(component.Image(label, options...))
}

func (blocks *Blocks) Audio(label string, options ...component.Option) component.Component {
	return blocks.add(component.Audio(label, options...))
}

func (blocks *Blocks) File(label string, options ...component.Option) component.Component {
	return blocks.add(component.File(label, options...))
}

func (blocks *Blocks) State(label string, options ...component.Option) component.Component {
	return blocks.add(component.State(label, options...))
}

func (blocks *Blocks) add(item component.Component) component.Component {
	item.ID = blocks.id + "-component-" + strconv.Itoa(len(blocks.components)+1)
	item = component.WithEventBinder(item, blocks)
	blocks.components = append(blocks.components, item)
	return item
}

func (blocks *Blocks) Load(handler *runtime.HandlerBinding, outputs component.List) {
	blocks.events = append(blocks.events, EventBinding{
		ID:      blocks.id + "-event-" + strconv.Itoa(len(blocks.events)+1),
		Trigger: "load",
		Outputs: componentIDs(outputs.Components),
		Handler: handler,
	})
}

func (blocks *Blocks) BindEvent(trigger string, source component.Component, handler *runtime.HandlerBinding, inputs component.List, outputs component.List) {
	blocks.events = append(blocks.events, EventBinding{
		ID:      blocks.id + "-event-" + strconv.Itoa(len(blocks.events)+1),
		Trigger: trigger,
		Source:  source.ID,
		Inputs:  componentIDs(inputs.Components),
		Outputs: componentIDs(outputs.Components),
		Handler: handler,
	})
}

func componentIDs(components []component.Component) []string {
	ids := make([]string, 0, len(components))
	for _, item := range components {
		ids = append(ids, item.ID)
	}
	return ids
}
```

- [ ] **Step 5: Extend `core.Interface` and `core.App`**

Modify `core/app.go`:

```go
type Interface struct {
	ID         string                `json:"id"`
	Kind       string                `json:"kind"`
	Inputs     []component.Component `json:"inputs"`
	Outputs    []component.Component `json:"outputs"`
	Components []component.Component `json:"components,omitempty"`
	Events     []EventBinding        `json:"events,omitempty"`

	Handler      *runtime.HandlerBinding `json:"-"`
	VoiceHandler *runtime.VoiceBinding   `json:"-"`
}
```

Add method:

```go
func (app *App) Blocks(build func(*Blocks)) {
	app.mu.Lock()
	defer app.mu.Unlock()

	id := "blocks-" + strconv.Itoa(countKind(app.interfaces, "blocks")+1)
	blocks := newBlocks(id)
	if build != nil {
		build(blocks)
	}

	components := assignComponentIDs(blocks.components, id+"-component")
	app.states[id] = collectInitialStateValues(components)
	app.interfaces = append(app.interfaces, Interface{
		ID:         id,
		Kind:       "blocks",
		Inputs:     []component.Component{},
		Outputs:    []component.Component{},
		Components: components,
		Events:     append([]EventBinding{}, blocks.events...),
	})
}
```

Update `Schema()` clone:

```go
interfaces = append(interfaces, Interface{
	ID:         iface.ID,
	Kind:       iface.Kind,
	Inputs:     cloneComponents(iface.Inputs),
	Outputs:    cloneComponents(iface.Outputs),
	Components: cloneComponents(iface.Components),
	Events:     cloneEvents(iface.Events),
})
```

Add:

```go
func cloneEvents(events []EventBinding) []EventBinding {
	result := make([]EventBinding, 0, len(events))
	for _, event := range events {
		event.Inputs = append([]string{}, event.Inputs...)
		event.Outputs = append([]string{}, event.Outputs...)
		event.Handler = nil
		result = append(result, event)
	}
	return result
}

func (app *App) GetEvent(interfaceID, eventID string) (Interface, EventBinding, bool) {
	iface, ok := app.GetInterface(interfaceID)
	if !ok {
		return Interface{}, EventBinding{}, false
	}
	for _, event := range iface.Events {
		if event.ID == eventID {
			return iface, event, true
		}
	}
	return Interface{}, EventBinding{}, false
}
```

- [ ] **Step 6: Add public Blocks facade**

Create `blocks_facade.go`:

```go
package goleo

import "github.com/sneiko/goleo/core"

type Blocks = core.Blocks
type ComponentRef = Component
```

- [ ] **Step 7: Run tests**

Run:

```bash
GOCACHE=/tmp/goleo-gocache go test ./... -run 'TestBlocksSchemaIncludesComponentsAndEvents|TestUpdateHelpersBuildPointerFields'
```

Expected: PASS.

- [ ] **Step 8: Commit**

Run:

```bash
git add component/component.go core/blocks.go blocks_facade.go core/app.go app_test.go
git commit -m "feat: add blocks schema"
```

## Task 3: `/api/event` backend execution

**Files:**
- Modify: `server/server.go`
- Test: `app_test.go`

- [ ] **Step 1: Write failing endpoint tests**

Add to `app_test.go`:

```go
func TestEventEndpointInvokesBlocksHandler(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	app.Blocks(func(blocks *goleo.Blocks) {
		prompt := blocks.Textbox("Prompt")
		out := blocks.Textbox("Result")
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

	body := strings.NewReader(`{"interface_id":"blocks-1","event_id":"blocks-1-event-1","data":{"blocks-1-component-1":"hello"}}`)
	resp, err := http.Post(server.URL+"/api/event", "application/json", body)
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
	if got.Data["blocks-1-component-2"] != "HELLO" {
		t.Fatalf("data = %#v, want output HELLO", got.Data)
	}
}

func TestEventEndpointReturnsUpdateEnvelope(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	app.Blocks(func(blocks *goleo.Blocks) {
		prompt := blocks.Textbox("Prompt")
		run := blocks.Button("Run")

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

	body := strings.NewReader(`{"interface_id":"blocks-1","event_id":"blocks-1-event-1","data":{"blocks-1-component-1":""}}`)
	resp, err := http.Post(server.URL+"/api/event", "application/json", body)
	if err != nil {
		t.Fatalf("post event: %v", err)
	}
	defer resp.Body.Close()

	var got struct {
		Data map[string]map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode event response: %v", err)
	}
	update := got.Data["blocks-1-component-2"]
	if update["kind"] != "update" || update["disabled"] != true {
		t.Fatalf("update = %#v, want disabled update", update)
	}
}

func TestEventEndpointQueueLimitReturnsError(t *testing.T) {
	app := goleo.New()
	app.ConfigureQueue(1, 0)
	app.Blocks(func(blocks *goleo.Blocks) {
		input := blocks.Textbox("Prompt")
		output := blocks.Textbox("Result")
		run := blocks.Button("Run")
		run.Click(
			goleo.Handler(func(value string) (string, error) {
				time.Sleep(300 * time.Millisecond)
				return value, nil
			}),
			goleo.Inputs(input),
			goleo.Outputs(output),
		)
	})

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	body := func(value string) io.Reader {
		return strings.NewReader(`{"interface_id":"blocks-1","event_id":"blocks-1-event-1","data":{"blocks-1-component-1":"` + value + `"}}`)
	}

	firstDone := make(chan struct{})
	go func() {
		resp, err := http.Post(server.URL+"/api/event", "application/json", body("first"))
		if err == nil {
			_ = resp.Body.Close()
		}
		close(firstDone)
	}()

	time.Sleep(20 * time.Millisecond)
	resp, err := http.Post(server.URL+"/api/event", "application/json", body("second"))
	if err != nil {
		t.Fatalf("post second event: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusTooManyRequests)
	}
	<-firstDone
}

func TestEventEndpointSupportsCancellation(t *testing.T) {
	app := goleo.New()
	var cancelled int32
	app.Blocks(func(blocks *goleo.Blocks) {
		input := blocks.Textbox("Prompt")
		output := blocks.Textbox("Result")
		run := blocks.Button("Run")
		run.Click(
			goleo.Handler(func(ctx context.Context, value string) (string, error) {
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

	req, err := http.NewRequest(
		http.MethodPost,
		server.URL+"/api/event",
		strings.NewReader(`{"interface_id":"blocks-1","event_id":"blocks-1-event-1","data":{"blocks-1-component-1":"hello"}}`),
	)
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

	time.Sleep(50 * time.Millisecond)
	cancelResp, err := http.Post(server.URL+"/api/cancel", "application/json", strings.NewReader(`{"request_id":"blocks-cancel-test"}`))
	if err != nil {
		t.Fatalf("post cancel: %v", err)
	}
	defer cancelResp.Body.Close()

	if cancelResp.StatusCode != http.StatusOK {
		t.Fatalf("cancel status = %d, want %d", cancelResp.StatusCode, http.StatusOK)
	}
	<-done

	if atomic.LoadInt32(&cancelled) == 0 {
		t.Fatal("event handler was not cancelled")
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
GOCACHE=/tmp/goleo-gocache go test ./... -run 'TestEventEndpointInvokesBlocksHandler|TestEventEndpointReturnsUpdateEnvelope|TestEventEndpointQueueLimitReturnsError|TestEventEndpointSupportsCancellation'
```

Expected: FAIL with HTTP 404 for `/api/event`.

- [ ] **Step 3: Add event request/response structs and route**

Modify `server/server.go` near request structs:

```go
type eventRequest struct {
	InterfaceID string         `json:"interface_id"`
	EventID     string         `json:"event_id"`
	Data        map[string]any `json:"data"`
}

type eventResponse struct {
	Data map[string]any `json:"data"`
}
```

Register route in `New`:

```go
mux.HandleFunc("POST /api/event", handleEvent(app, store, queue, requestRegistry))
```

- [ ] **Step 4: Add event handler helpers**

Add to `server/server.go`:

```go
func handleEvent(app *core.App, store *assetStore, queue *queueManager, registry *requestRegistry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := app.Logger()
		rawContext := r.Context()
		handlerContext, handlerCancel := context.WithCancel(rawContext)
		requestID := requestIDFromContext(rawContext)
		if requestID == "" {
			requestID = newRequestID()
		}
		if registry != nil {
			registry.register(requestID, handlerCancel)
			defer registry.unregister(requestID)
		}

		request, err := decodeEventRequest(r)
		if err != nil {
			warnRequest(logger, r, "event request decode failed", "error", err)
			writeError(w, http.StatusBadRequest, err)
			return
		}

		iface, event, ok := app.GetEvent(request.InterfaceID, request.EventID)
		if !ok || iface.Kind != "blocks" {
			err := fmt.Errorf("event %q not found", request.EventID)
			warnRequest(logger, r, "event binding not found", "interface_id", request.InterfaceID, "event_id", request.EventID, "error", err)
			writeError(w, http.StatusNotFound, err)
			return
		}
		if event.Handler == nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("event %q does not have handler", event.ID))
			return
		}

		release, _, err := queue.acquire(handlerContext, iface.ID)
		if errors.Is(err, errQueueFull) {
			writeError(w, http.StatusTooManyRequests, errQueueFull)
			return
		}
		if err != nil {
			writeError(w, http.StatusRequestTimeout, err)
			return
		}
		defer release()

		inputComponents := componentsByIDs(iface.Components, event.Inputs)
		outputComponents := componentsByIDs(iface.Components, event.Outputs)
		requestData := valuesForComponents(inputComponents, request.Data)
		requestData = mergeStateInputs(handlerContext, app, iface.ID, inputComponents, requestData)

		hydrated, err := hydrateAssets(inputComponents, requestData, store)
		if err != nil {
			warnRequest(logger, r, "event asset hydration failed", "interface_id", request.InterfaceID, "event_id", request.EventID, "error", err)
			writeError(w, http.StatusBadRequest, err)
			return
		}

		values, err := event.Handler.Invoke(handlerContext, hydrated)
		if err != nil {
			status := http.StatusBadRequest
			if isPanicError(err) {
				status = http.StatusInternalServerError
				errorRequest(logger, r, "event handler panicked", "interface_id", request.InterfaceID, "event_id", request.EventID, "error", err)
			} else {
				warnRequest(logger, r, "event handler failed", "interface_id", request.InterfaceID, "event_id", request.EventID, "error", err)
			}
			writeError(w, status, err)
			return
		}

		responseData, err := dehydrateEventOutputs(outputComponents, values, store)
		if err != nil {
			errorRequest(logger, r, "event output dehydration failed", "interface_id", request.InterfaceID, "event_id", request.EventID, "error", err)
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		updateStateFromOutputs(app, iface.ID, outputComponents, values)

		writeJSON(w, http.StatusOK, eventResponse{Data: mapOutputsByID(outputComponents, responseData)})
	}
}
```

Add helper functions:

```go
func decodeEventRequest(r *http.Request) (eventRequest, error) {
	defer r.Body.Close()

	var request eventRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		return eventRequest{}, err
	}
	if request.InterfaceID == "" {
		return eventRequest{}, errors.New("interface_id is required")
	}
	if request.EventID == "" {
		return eventRequest{}, errors.New("event_id is required")
	}
	if request.Data == nil {
		request.Data = map[string]any{}
	}
	return request, nil
}

func componentsByIDs(components []component.Component, ids []string) []component.Component {
	flat := flattenLeafComponents(components)
	byID := make(map[string]component.Component, len(flat))
	for _, item := range flat {
		byID[item.ID] = item
	}
	result := make([]component.Component, 0, len(ids))
	for _, id := range ids {
		if item, ok := byID[id]; ok {
			result = append(result, item)
		}
	}
	return result
}

func valuesForComponents(components []component.Component, values map[string]any) []any {
	result := make([]any, 0, len(components))
	for _, item := range components {
		result = append(result, values[item.ID])
	}
	return result
}

func mapOutputsByID(components []component.Component, values []any) map[string]any {
	result := make(map[string]any, len(components))
	for index, item := range components {
		if index < len(values) {
			result[item.ID] = values[index]
		} else {
			result[item.ID] = nil
		}
	}
	return result
}
```

- [ ] **Step 5: Add update envelope support**

Add to `server/server.go`:

```go
func dehydrateEventOutputs(components []component.Component, values []any, store *assetStore) ([]any, error) {
	result := make([]any, 0, len(values))
	for index, value := range values {
		if update, ok := value.(runtime.Update); ok {
			componentForUpdate := []component.Component{}
			if index < len(components) {
				componentForUpdate = append(componentForUpdate, components[index])
			}
			envelope, err := updateEnvelope(update, componentForUpdate, store)
			if err != nil {
				return nil, err
			}
			result = append(result, envelope)
			continue
		}
		dehydrated, err := dehydrateOutputs(componentAt(components, index), []any{value}, store)
		if err != nil {
			return nil, err
		}
		if len(dehydrated) == 0 {
			result = append(result, value)
		} else {
			result = append(result, dehydrated[0])
		}
	}
	return result, nil
}

func componentAt(components []component.Component, index int) []component.Component {
	if index < 0 || index >= len(components) {
		return nil
	}
	return []component.Component{components[index]}
}

func updateEnvelope(update runtime.Update, components []component.Component, store *assetStore) (map[string]any, error) {
	payload := map[string]any{"kind": runtime.UpdateKind}
	if update.Value != nil {
		value := update.Value
		if len(components) > 0 {
			dehydrated, err := dehydrateOutputs(components, []any{update.Value}, store)
			if err != nil {
				return nil, err
			}
			if len(dehydrated) > 0 {
				value = dehydrated[0]
			}
		}
		payload["value"] = value
	}
	if update.Visible != nil {
		payload["visible"] = *update.Visible
	}
	if update.Disabled != nil {
		payload["disabled"] = *update.Disabled
	}
	if update.Choices != nil {
		payload["choices"] = append([]string{}, update.Choices...)
	}
	if update.Label != nil {
		payload["label"] = *update.Label
	}
	return payload, nil
}
```

- [ ] **Step 6: Run backend tests**

Run:

```bash
GOCACHE=/tmp/goleo-gocache go test ./... -run 'TestEventEndpointInvokesBlocksHandler|TestEventEndpointReturnsUpdateEnvelope|TestEventEndpointQueueLimitReturnsError|TestEventEndpointSupportsCancellation'
```

Expected: PASS.

- [ ] **Step 7: Commit**

Run:

```bash
git add server/server.go app_test.go
git commit -m "feat: execute blocks events"
```

## Task 4: Blocks frontend API client and types

**Files:**
- Modify: `frontend/src/types.ts`
- Modify: `frontend/src/lib/api.ts`
- Modify: `frontend/src/lib/api.test.ts`

- [ ] **Step 1: Write failing API client test**

Add to `frontend/src/lib/api.test.ts`:

```ts
import { sendEvent } from "./api";

it("posts blocks event requests using component value maps", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_url, _init) => {
      return new Response(JSON.stringify({ data: { "blocks-1-component-2": "HELLO" } }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );

  const result = await sendEvent("blocks-1", "blocks-1-event-1", {
    "blocks-1-component-1": "hello",
  });

  expect(result).toEqual({ "blocks-1-component-2": "HELLO" });
  expect(fetch).toHaveBeenCalledWith("/api/event", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      interface_id: "blocks-1",
      event_id: "blocks-1-event-1",
      data: { "blocks-1-component-1": "hello" },
    }),
  });
});
```

- [ ] **Step 2: Run test to verify failure**

Run:

```bash
cd frontend && pnpm test -- --run src/lib/api.test.ts
```

Expected: FAIL because `sendEvent` is not exported.

- [ ] **Step 3: Add frontend types**

Modify `frontend/src/types.ts`:

```ts
export type EventSchema = {
  id: string;
  trigger: "click" | "change" | "load" | string;
  source?: string;
  inputs: string[];
  outputs: string[];
};

export type ComponentUpdate = {
  kind: "update";
  value?: unknown;
  visible?: boolean;
  disabled?: boolean;
  choices?: string[];
  label?: string;
};
```

Extend `InterfaceSchema`:

```ts
components?: ComponentSchema[];
events?: EventSchema[];
```

- [ ] **Step 4: Add `sendEvent`**

Modify `frontend/src/lib/api.ts`:

```ts
export async function sendEvent(
  interfaceID: string,
  eventID: string,
  data: Record<string, unknown>,
): Promise<Record<string, unknown>> {
  const response = await fetch("/api/event", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ interface_id: interfaceID, event_id: eventID, data }),
  });
  if (!response.ok) {
    throw new Error(await readErrorMessage(response));
  }

  const payload = (await response.json()) as { data?: Record<string, unknown> };
  return payload.data ?? {};
}
```

- [ ] **Step 5: Run API tests**

Run:

```bash
cd frontend && pnpm test -- --run src/lib/api.test.ts
```

Expected: PASS.

- [ ] **Step 6: Commit**

Run:

```bash
git add frontend/src/types.ts frontend/src/lib/api.ts frontend/src/lib/api.test.ts
git commit -m "feat: add blocks event api client"
```

## Task 5: Blocks frontend renderer

**Files:**
- Modify: `frontend/src/App.tsx`
- Modify: `frontend/src/App.test.tsx`

- [ ] **Step 1: Write failing frontend tests**

Add to `frontend/src/App.test.tsx`:

```tsx
it("renders blocks components and dispatches click events", async () => {
  mockedAPI.loadSchema.mockResolvedValue(blocksSchema());
  mockedAPI.sendEvent.mockResolvedValue({ "blocks-1-component-2": "HELLO" });
  const user = userEvent.setup();

  render(<App />);

  await user.type(await screen.findByLabelText("Prompt"), "hello");
  await user.click(screen.getByRole("button", { name: "Run" }));

  expect(mockedAPI.sendEvent).toHaveBeenCalledWith("blocks-1", "blocks-1-event-1", {
    "blocks-1-component-1": "hello",
    "blocks-1-component-2": "",
    "blocks-1-component-3": "",
  });
  expect(await screen.findByDisplayValue("HELLO")).toBeInTheDocument();
});

it("applies blocks update envelopes", async () => {
  mockedAPI.loadSchema.mockResolvedValue(blocksSchema());
  mockedAPI.sendEvent.mockResolvedValue({
    "blocks-1-component-3": { kind: "update", disabled: true, label: "Waiting" },
  });
  const user = userEvent.setup();

  render(<App />);

  await user.type(await screen.findByLabelText("Prompt"), "x");

  expect(mockedAPI.sendEvent).toHaveBeenCalledWith(
    "blocks-1",
    "blocks-1-event-2",
    expect.objectContaining({ "blocks-1-component-1": "x" }),
  );
  expect(await screen.findByRole("button", { name: "Waiting" })).toBeDisabled();
});

it("dispatches blocks load events once", async () => {
  mockedAPI.loadSchema.mockResolvedValue(blocksSchema());
  mockedAPI.sendEvent.mockResolvedValue({ "blocks-1-component-2": "Loaded" });

  render(<App />);

  expect(await screen.findByDisplayValue("Loaded")).toBeInTheDocument();
  expect(mockedAPI.sendEvent).toHaveBeenCalledWith("blocks-1", "blocks-1-event-3", {
    "blocks-1-component-1": "",
    "blocks-1-component-2": "",
    "blocks-1-component-3": "",
  });
});
```

Add helper:

```tsx
function blocksSchema(): AppSchema {
  return {
    version: "0.1.0",
    interfaces: [
      {
        id: "blocks-1",
        kind: "blocks",
        inputs: [],
        outputs: [],
        components: [
          { id: "blocks-1-component-1", type: "textbox", label: "Prompt", props: {} },
          { id: "blocks-1-component-2", type: "textbox", label: "Result", props: {} },
          { id: "blocks-1-component-3", type: "button", label: "Run", props: {} },
        ],
        events: [
          {
            id: "blocks-1-event-1",
            trigger: "click",
            source: "blocks-1-component-3",
            inputs: ["blocks-1-component-1"],
            outputs: ["blocks-1-component-2"],
          },
          {
            id: "blocks-1-event-2",
            trigger: "change",
            source: "blocks-1-component-1",
            inputs: ["blocks-1-component-1"],
            outputs: ["blocks-1-component-3"],
          },
          {
            id: "blocks-1-event-3",
            trigger: "load",
            inputs: [],
            outputs: ["blocks-1-component-2"],
          },
        ],
      },
    ],
  };
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
cd frontend && pnpm test -- --run src/App.test.tsx
```

Expected: FAIL because `sendEvent` mock is missing and `kind=blocks` renders as `FormInterface`.

- [ ] **Step 3: Import types/API and route blocks kind**

Modify imports in `frontend/src/App.tsx`:

```ts
import { loadSchema, openVoiceSession, cancelRequest, sendEvent, streamWithEvents, uploadFile, predict } from "@/lib/api";
import type { AppSchema, ComponentSchema, ComponentUpdate, EventSchema, InterfaceSchema, StreamEvent, UploadResponse, VoiceServerEvent, VoiceSessionConnection } from "@/types";
```

Modify root renderer:

```tsx
) : iface.kind === "blocks" ? (
  <BlocksInterface key={iface.id} iface={iface} />
) : (
```

- [ ] **Step 4: Add button rendering support**

Modify `SchemaInput` signature:

```tsx
onAction?: (component: ComponentSchema) => void;
```

Before media branches, add:

```tsx
if (component.type === "button") {
  return (
    <Field data-disabled={disabled || undefined}>
      <Button
        type="button"
        disabled={disabled || props.disabled === true}
        onClick={() => onAction?.(component)}
      >
        {component.label}
      </Button>
    </Field>
  );
}
```

Propagate `onAction` through `renderSchemaInputs`.

- [ ] **Step 5: Add BlocksInterface**

Add to `frontend/src/App.tsx`:

```tsx
function BlocksInterface({ iface }: { iface: InterfaceSchema }) {
  const [components, setComponents] = useState<ComponentSchema[]>(() => iface.components ?? []);
  const leafComponents = useMemo(() => flattenLeafComponents(components), [components]);
  const [values, setValues] = useInitialValues(leafComponents);
  const [error, setError] = useState<string | null>(null);
  const [pendingEventID, setPendingEventID] = useState<string | null>(null);
  const events = iface.events ?? [];

  async function runEvent(event: EventSchema, nextValues: Values = values) {
    setError(null);
    setPendingEventID(event.id);
    try {
      const result = await sendEvent(iface.id, event.id, nextValues);
      applyEventResult(result);
    } catch (runError) {
      setError(errorMessage(runError));
    } finally {
      setPendingEventID(null);
    }
  }

  function applyEventResult(result: Record<string, unknown>) {
    setValues((current) => {
      const next = { ...current };
      for (const [componentID, value] of Object.entries(result)) {
        if (isComponentUpdate(value)) {
          if ("value" in value) {
            next[componentID] = value.value;
          }
        } else {
          next[componentID] = value;
        }
      }
      return next;
    });

    setComponents((current) => applyComponentUpdates(current, result));
  }

  function handleChange(component: ComponentSchema, value: unknown) {
    const nextValues = { ...values, [component.id]: value };
    setValues(nextValues);
    for (const event of events.filter((item) => item.trigger === "change" && item.source === component.id)) {
      void runEvent(event, nextValues);
    }
  }

  function handleAction(component: ComponentSchema) {
    for (const event of events.filter((item) => item.trigger === "click" && item.source === component.id)) {
      void runEvent(event);
    }
  }

  useEffect(() => {
    for (const event of events.filter((item) => item.trigger === "load")) {
      void runEvent(event);
    }
  }, []);

  return (
    <Card className="overflow-hidden border-border/80 shadow-sm">
      <CardHeader className="border-b bg-card/70">
        <CardTitle>Blocks</CardTitle>
        <CardDescription>{iface.id}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-5 pt-6">
        <FieldGroup className="gap-5">
          {renderSchemaInputs(components, Boolean(pendingEventID), values, handleChange, handleAction)}
        </FieldGroup>
        {error ? <ErrorAlert title="Event failed" message={error} /> : null}
      </CardContent>
    </Card>
  );
}
```

Add helpers:

```tsx
function isComponentUpdate(value: unknown): value is ComponentUpdate {
  return typeof value === "object" && value !== null && (value as { kind?: unknown }).kind === "update";
}

function applyComponentUpdates(components: ComponentSchema[], result: Record<string, unknown>): ComponentSchema[] {
  return components.map((component) => {
    const raw = result[component.id];
    const items = component.items ? applyComponentUpdates(component.items, result) : component.items;
    if (!isComponentUpdate(raw)) {
      return items === component.items ? component : { ...component, items };
    }

    const props = { ...(component.props ?? {}) };
    if (raw.visible !== undefined) {
      props.visible = raw.visible;
    }
    if (raw.disabled !== undefined) {
      props.disabled = raw.disabled;
    }
    return {
      ...component,
      label: raw.label ?? component.label,
      choices: raw.choices ?? component.choices,
      props,
      items,
    };
  });
}
```

- [ ] **Step 6: Update mock typing**

`vi.mock("./lib/api")` will expose `sendEvent` automatically after Task 4. If TypeScript complains, import `sendEvent` through `* as api` is already used and `mockedAPI.sendEvent` will typecheck after export.

- [ ] **Step 7: Run frontend tests**

Run:

```bash
cd frontend && pnpm test -- --run src/App.test.tsx
```

Expected: PASS.

- [ ] **Step 8: Commit**

Run:

```bash
git add frontend/src/App.tsx frontend/src/App.test.tsx
git commit -m "feat: render blocks events in frontend"
```

## Task 6: Example and documentation

**Files:**
- Create: `examples/blocks/main.go`
- Modify: `Makefile`
- Modify: `README.md`
- Modify: `docs/usage.md`
- Modify: `docs/components.md`
- Modify: `docs/architecture.md`

- [ ] **Step 1: Create runnable example**

Create `examples/blocks/main.go`:

```go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/sneiko/goleo"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	app := goleo.New(goleo.WithLogger(logger))

	app.Blocks(func(blocks *goleo.Blocks) {
		prompt := blocks.Textbox("Prompt", goleo.WithPlaceholder("Describe the demo"))
		style := blocks.Dropdown("Style", "concise", "detailed", "playful")
		run := blocks.Button("Run")
		result := blocks.Textbox("Result", goleo.WithRows(5))

		blocks.Load(
			goleo.Handler(func() (string, error) {
				return "Ready", nil
			}),
			goleo.Outputs(result),
		)

		prompt.Change(
			goleo.Handler(func(value string) (goleo.Update, error) {
				return goleo.Disabled(strings.TrimSpace(value) == ""), nil
			}),
			goleo.Inputs(prompt),
			goleo.Outputs(run),
		)

		run.Click(
			goleo.Handler(func(prompt string, style string) (string, error) {
				return fmt.Sprintf("[%s] %s", style, strings.TrimSpace(prompt)), nil
			}),
			goleo.Inputs(prompt, style),
			goleo.Outputs(result),
		)
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	addr := os.Getenv("GOLEO_ADDR")
	if addr == "" {
		addr = ":7860"
	}
	if err := app.LaunchContext(ctx, goleo.LaunchOptions{Addr: addr}); err != nil {
		logger.Error("goleo server stopped", "error", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: Add Makefile target**

Add:

```make
.PHONY: run-blocks
run-blocks:
	GOLEO_ADDR=$${GOLEO_ADDR:-:7871} go run ./examples/blocks
```

- [ ] **Step 3: Update docs**

Add README section:

```md
## Interface vs Blocks

Use `Interface` when one submit button maps a fixed input list to fixed outputs.
Use `Blocks` when UI components need their own events: click, change, load, and
partial component updates.

```go
app.Blocks(func(blocks *goleo.Blocks) {
	prompt := blocks.Textbox("Prompt")
	run := blocks.Button("Run")
	out := blocks.Textbox("Result")
	run.Click(goleo.Handler(func(input string) string {
		return strings.ToUpper(input)
	}), goleo.Inputs(prompt), goleo.Outputs(out))
})
```
```

Update `docs/usage.md` with `/api/event`, `Blocks`, and `Update`.

Update `docs/components.md` to explain `ComponentRef`, button events, and update envelope.

Update `docs/architecture.md` to describe `kind=blocks` and event execution.

- [ ] **Step 4: Run example compile**

Run:

```bash
GOCACHE=/tmp/goleo-gocache go test ./examples/blocks
```

Expected: package compiles, no tests.

- [ ] **Step 5: Commit**

Run:

```bash
git add examples/blocks/main.go Makefile README.md docs/usage.md docs/components.md docs/architecture.md
git commit -m "docs: add blocks example and usage docs"
```

## Task 7: Full verification

**Files:**
- No code files unless verification finds a defect.

- [ ] **Step 1: Run Go tests**

Run:

```bash
GOCACHE=/tmp/goleo-gocache go test ./...
```

Expected: PASS. If sandbox blocks `httptest` bind with `operation not permitted`, rerun normal `go test ./...` outside restricted sandbox or record the exact sandbox failure.

- [ ] **Step 2: Run frontend tests**

Run:

```bash
cd frontend && pnpm test -- --run
```

Expected: PASS.

- [ ] **Step 3: Check git state**

Run:

```bash
git status --short
git log -5 --oneline
```

Expected: clean working tree after commits; latest commits include Blocks/Event work.

- [ ] **Step 4: Final commit if verification fixes were needed**

If verification required fixes, commit them:

```bash
git add <fixed-files>
git commit -m "fix: stabilize blocks event api"
```

Expected: clean working tree.

## Self-review

- Spec coverage: `Blocks`, `ComponentRef`, `Update`, schema `kind=blocks`, `/api/event`, queue/cancel reuse, frontend click/change/load, docs/example, and tests are covered.
- Scope: chaining, tabs, batch, streaming blocks, durable multi-user state, and client parity are intentionally outside this plan.
- Type consistency: public API uses `goleo.Blocks`, `goleo.ComponentRef`, `goleo.Update`; schema uses `components` and `events`; frontend update envelope uses `kind: "update"`.
