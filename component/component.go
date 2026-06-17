package component

import "strings"

// Component describes a UI control that the frontend renderer can mount.
type Component struct {
	ID      string         `json:"id"`
	Type    string         `json:"type"`
	Label   string         `json:"label"`
	Props   map[string]any `json:"props"`
	Choices []string       `json:"choices,omitempty"`
}

// Option customizes component metadata.
type Option func(*Component)

// WithProp adds an arbitrary renderer property to a component.
func WithProp(key string, value any) Option {
	return func(component *Component) {
		component.Props[key] = value
	}
}

// WithChoices sets selectable options for dropdown-like components.
func WithChoices(choices ...string) Option {
	return func(component *Component) {
		component.Choices = append([]string{}, choices...)
	}
}

// WithPlaceholder sets placeholder text for text-like inputs.
func WithPlaceholder(value string) Option {
	return WithProp("placeholder", value)
}

// WithDefault sets the initial component value.
func WithDefault(value any) Option {
	return WithProp("default", value)
}

// WithMin sets the minimum accepted numeric value.
func WithMin(value float64) Option {
	return WithProp("min", value)
}

// WithMax sets the maximum accepted numeric value.
func WithMax(value float64) Option {
	return WithProp("max", value)
}

// WithStep sets the numeric input step.
func WithStep(value float64) Option {
	return WithProp("step", value)
}

// WithRows sets the visible row count for multiline text inputs.
func WithRows(value int) Option {
	return WithProp("rows", value)
}

// WithDisabled marks a component as disabled.
func WithDisabled(value bool) Option {
	return WithProp("disabled", value)
}

// WithVisible controls whether a component is initially visible.
func WithVisible(value bool) Option {
	return WithProp("visible", value)
}

// WithAccept sets accepted MIME types or extensions for file-like inputs.
func WithAccept(value string) Option {
	return WithProp("accept", value)
}

// WithMultiple controls whether file-like inputs accept multiple files.
func WithMultiple(value bool) Option {
	return WithProp("multiple", value)
}

func New(kind string, label string, options ...Option) Component {
	component := Component{
		Type:  strings.ToLower(kind),
		Label: label,
		Props: map[string]any{},
	}

	for _, option := range options {
		option(&component)
	}

	return component
}

func Textbox(label string, options ...Option) Component {
	return New("textbox", label, options...)
}

func Number(label string, options ...Option) Component {
	return New("number", label, options...)
}

func Slider(label string, options ...Option) Component {
	return New("slider", label, options...)
}

func Checkbox(label string, options ...Option) Component {
	return New("checkbox", label, options...)
}

func Dropdown(label string, choices ...string) Component {
	return New("dropdown", label, WithChoices(choices...))
}

func Button(label string, options ...Option) Component {
	return New("button", label, options...)
}

func Markdown(label string, options ...Option) Component {
	return New("markdown", label, options...)
}

func JSON(label string, options ...Option) Component {
	return New("json", label, options...)
}

func Image(label string, options ...Option) Component {
	return New("image", label, options...)
}

func Audio(label string, options ...Option) Component {
	return New("audio", label, options...)
}

func File(label string, options ...Option) Component {
	return New("file", label, options...)
}

func Chatbot(label string, options ...Option) Component {
	return New("chatbot", label, options...)
}

// List groups components passed to Interface.
type List struct {
	Components []Component
}

func Inputs(components ...Component) List {
	return List{Components: append([]Component{}, components...)}
}

func Outputs(components ...Component) List {
	return List{Components: append([]Component{}, components...)}
}
