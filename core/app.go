package core

import (
	"context"
	"log/slog"
	"strconv"
	"sync"

	"github.com/sneiko/goleo/component"
	"github.com/sneiko/goleo/runtime"
)

const schemaVersion = "0.1.0"

var defaultLogger = slog.New(discardHandler{})

type discardHandler struct{}

func (discardHandler) Enabled(context.Context, slog.Level) bool {
	return false
}

func (discardHandler) Handle(context.Context, slog.Record) error {
	return nil
}

func (handler discardHandler) WithAttrs([]slog.Attr) slog.Handler {
	return handler
}

func (handler discardHandler) WithGroup(string) slog.Handler {
	return handler
}

// App is a runnable AI demo application model.
type App struct {
	mu         sync.RWMutex
	interfaces []Interface
	logger     *slog.Logger

	stateMu      sync.RWMutex
	states       map[string]map[string]any
	predictQueue queuePolicy
}

// Interface describes one callable UI surface.
type Interface struct {
	ID      string                `json:"id"`
	Kind    string                `json:"kind"`
	Inputs  []component.Component `json:"inputs"`
	Outputs []component.Component `json:"outputs"`

	Handler      *runtime.HandlerBinding `json:"-"`
	VoiceHandler *runtime.VoiceBinding   `json:"-"`
}

type queuePolicy struct {
	MaxConcurrency int
	MaxQueue       int
}

// Schema is the JSON contract consumed by the embedded frontend.
type Schema struct {
	Version    string      `json:"version"`
	Interfaces []Interface `json:"interfaces"`
}

// New creates an empty app model.
func New() *App {
	return &App{
		interfaces: []Interface{},
		logger:     defaultLogger,
		states:     map[string]map[string]any{},
		predictQueue: queuePolicy{
			MaxConcurrency: 2,
			MaxQueue:       64,
		},
	}
}

// SetLogger configures structured logging for the app.
func (app *App) SetLogger(logger *slog.Logger) {
	if logger == nil {
		return
	}

	app.mu.Lock()
	defer app.mu.Unlock()

	app.logger = logger
}

// Logger returns the configured structured logger.
func (app *App) Logger() *slog.Logger {
	app.mu.RLock()
	defer app.mu.RUnlock()

	if app.logger == nil {
		return defaultLogger
	}

	return app.logger
}

// ConfigureQueue changes prediction/stream queue behavior.
func (app *App) ConfigureQueue(maxConcurrency, maxQueue int) {
	app.mu.Lock()
	defer app.mu.Unlock()

	if maxConcurrency <= 0 {
		maxConcurrency = 1
	}
	if maxQueue < 0 {
		maxQueue = 0
	}

	app.predictQueue = queuePolicy{
		MaxConcurrency: maxConcurrency,
		MaxQueue:       maxQueue,
	}
}

func (app *App) queuePolicy() queuePolicy {
	app.mu.RLock()
	defer app.mu.RUnlock()

	return app.predictQueue
}

func (app *App) State(ifaceID, componentID string) (any, bool) {
	app.stateMu.RLock()
	defer app.stateMu.RUnlock()

	ifaceState, ok := app.states[ifaceID]
	if !ok {
		return nil, false
	}

	value, ok := ifaceState[componentID]
	return value, ok
}

func (app *App) setState(ifaceID, componentID string, value any) {
	app.stateMu.Lock()
	defer app.stateMu.Unlock()

	ifaceState := app.states[ifaceID]
	if ifaceState == nil {
		ifaceState = map[string]any{}
		app.states[ifaceID] = ifaceState
	}
	ifaceState[componentID] = value
}

func (app *App) resetState(ifaceID string) {
	app.stateMu.Lock()
	defer app.stateMu.Unlock()

	delete(app.states, ifaceID)
}

// Interface registers a function-backed form interface.
func (app *App) Interface(
	handler *runtime.HandlerBinding,
	inputs component.List,
	outputs component.List,
) {
	id := "interface-" + strconv.Itoa(len(app.interfaces)+1)
	inputComponents := assignComponentIDs(inputs.Components, id+"-input")
	outputComponents := assignComponentIDs(outputs.Components, id+"-output")
	stateValues := collectInitialStateValues(inputComponents)
	app.mu.Lock()
	defer app.mu.Unlock()

	if len(stateValues) > 0 {
		app.states[id] = stateValues
	} else {
		app.states[id] = map[string]any{}
	}

	app.interfaces = append(app.interfaces, Interface{
		ID:      id,
		Kind:    "interface",
		Inputs:  inputComponents,
		Outputs: outputComponents,
		Handler: handler,
	})
}

// Chat registers a streaming chat interface.
func (app *App) Chat(handler *runtime.HandlerBinding) {
	app.mu.Lock()
	defer app.mu.Unlock()

	id := "chat-" + strconv.Itoa(countKind(app.interfaces, "chat")+1)
	inputs := assignComponentIDs([]component.Component{component.Textbox("Message")}, id+"-input")
	outputs := assignComponentIDs([]component.Component{component.Chatbot("Chat")}, id+"-output")
	app.states[id] = map[string]any{}
	app.interfaces = append(app.interfaces, Interface{
		ID:      id,
		Kind:    "chat",
		Inputs:  inputs,
		Outputs: outputs,
		Handler: handler,
	})
}

// Voice registers a bidirectional voice session interface.
func (app *App) Voice(handler *runtime.VoiceBinding) {
	app.mu.Lock()
	defer app.mu.Unlock()

	id := "voice-" + strconv.Itoa(countKind(app.interfaces, "voice")+1)
	app.states[id] = map[string]any{}
	app.interfaces = append(app.interfaces, Interface{
		ID:           id,
		Kind:         "voice",
		Inputs:       []component.Component{},
		Outputs:      []component.Component{},
		VoiceHandler: handler,
	})
}

// Schema returns a frontend-safe copy of the registered UI schema.
func (app *App) Schema() Schema {
	app.mu.RLock()
	defer app.mu.RUnlock()

	interfaces := make([]Interface, 0, len(app.interfaces))
	for _, iface := range app.interfaces {
		interfaces = append(interfaces, Interface{
			ID:      iface.ID,
			Kind:    iface.Kind,
			Inputs:  cloneComponents(iface.Inputs),
			Outputs: cloneComponents(iface.Outputs),
		})
	}

	return Schema{
		Version:    schemaVersion,
		Interfaces: interfaces,
	}
}

// GetInterface finds a registered interface by ID.
func (app *App) GetInterface(id string) (Interface, bool) {
	app.mu.RLock()
	defer app.mu.RUnlock()

	for _, iface := range app.interfaces {
		if iface.ID == id {
			return iface, true
		}
	}

	return Interface{}, false
}

func (app *App) StateForInput(ifaceID, componentID string) (any, bool) {
	return app.State(ifaceID, componentID)
}

func (app *App) SetState(ifaceID, componentID string, value any) {
	app.setState(ifaceID, componentID, value)
}

func (app *App) QueuePolicy() (maxConcurrency, maxQueue int) {
	policy := app.queuePolicy()
	return policy.MaxConcurrency, policy.MaxQueue
}

func assignComponentIDs(components []component.Component, prefix string) []component.Component {
	result := make([]component.Component, 0, len(components))
	for index, item := range components {
		if item.Props == nil {
			item.Props = map[string]any{}
		}
		if item.ID == "" {
			item.ID = prefix + "-" + strconv.Itoa(index+1)
		}
		if len(item.Items) > 0 {
			item.Items = assignComponentIDs(item.Items, item.ID)
		}
		result = append(result, item)
	}

	return result
}

func collectInitialStateValues(components []component.Component) map[string]any {
	state := map[string]any{}
	collectStateValues(components, state)
	return state
}

func collectStateValues(components []component.Component, state map[string]any) {
	for _, item := range components {
		if item.Type == "state" {
			if _, exists := item.Props["default"]; exists {
				state[item.ID] = item.Props["default"]
			}
		}

		if len(item.Items) > 0 {
			collectStateValues(item.Items, state)
		}
	}
}

func cloneComponents(components []component.Component) []component.Component {
	result := make([]component.Component, 0, len(components))
	for _, item := range components {
		props := make(map[string]any, len(item.Props))
		for key, value := range item.Props {
			props[key] = value
		}
		item.Props = props
		item.Items = cloneComponents(item.Items)
		item.Choices = append([]string{}, item.Choices...)
		result = append(result, item)
	}

	return result
}

func countKind(interfaces []Interface, kind string) int {
	var count int
	for _, iface := range interfaces {
		if iface.Kind == kind {
			count++
		}
	}

	return count
}
