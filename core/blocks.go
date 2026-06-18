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
