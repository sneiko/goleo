package goleo

import "github.com/sneiko/goleo/component"

type Component = component.Component
type ComponentLike = component.ComponentLike
type ComponentOption = component.Option
type ComponentList = component.List

func WithProp(key string, value any) ComponentOption {
	return component.WithProp(key, value)
}

func WithChoices(choices ...string) ComponentOption {
	return component.WithChoices(choices...)
}

func WithPlaceholder(value string) ComponentOption {
	return component.WithPlaceholder(value)
}

func WithDefault(value any) ComponentOption {
	return component.WithDefault(value)
}

func WithMin(value float64) ComponentOption {
	return component.WithMin(value)
}

func WithMax(value float64) ComponentOption {
	return component.WithMax(value)
}

func WithStep(value float64) ComponentOption {
	return component.WithStep(value)
}

func WithRows(value int) ComponentOption {
	return component.WithRows(value)
}

func WithDisabled(value bool) ComponentOption {
	return component.WithDisabled(value)
}

func WithVisible(value bool) ComponentOption {
	return component.WithVisible(value)
}

func WithAccept(value string) ComponentOption {
	return component.WithAccept(value)
}

func WithMultiple(value bool) ComponentOption {
	return component.WithMultiple(value)
}

func CustomComponent(kind string, label string, options ...ComponentOption) Component {
	return component.New(kind, label, options...)
}

func Textbox(label string, options ...ComponentOption) Component {
	return component.Textbox(label, options...)
}

func Number(label string, options ...ComponentOption) Component {
	return component.Number(label, options...)
}

func Slider(label string, options ...ComponentOption) Component {
	return component.Slider(label, options...)
}

func Checkbox(label string, options ...ComponentOption) Component {
	return component.Checkbox(label, options...)
}

func Dropdown(label string, choices ...string) Component {
	return component.Dropdown(label, choices...)
}

func Button(label string, options ...ComponentOption) Component {
	return component.Button(label, options...)
}

func Markdown(label string, options ...ComponentOption) Component {
	return component.Markdown(label, options...)
}

func JSON(label string, options ...ComponentOption) Component {
	return component.JSON(label, options...)
}

func Image(label string, options ...ComponentOption) Component {
	return component.Image(label, options...)
}

func Audio(label string, options ...ComponentOption) Component {
	return component.Audio(label, options...)
}

func File(label string, options ...ComponentOption) Component {
	return component.File(label, options...)
}

func State(label string, options ...ComponentOption) Component {
	return component.State(label, options...)
}

func Row(components ...Component) Component {
	return component.Row(components...)
}

func Column(components ...Component) Component {
	return component.Column(components...)
}

func Group(label string, components ...Component) Component {
	return component.Group(label, components...)
}

func Chatbot(label string, options ...ComponentOption) Component {
	return component.Chatbot(label, options...)
}

func Inputs(components ...ComponentLike) ComponentList {
	return component.Inputs(components...)
}

func Outputs(components ...ComponentLike) ComponentList {
	return component.Outputs(components...)
}
