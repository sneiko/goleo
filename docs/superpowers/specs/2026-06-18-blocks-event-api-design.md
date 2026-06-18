# Blocks/Event API MVP Design

## Цель

Приблизить Goleo к Gradio по самому важному архитектурному признаку: добавить низкоуровневую поверхность для композиции UI и событий, аналогичную базовой роли `gr.Blocks`.

Первый цикл намеренно ограничен MVP:

- компоненты объявляются как refs внутри `app.Blocks`;
- события привязываются к компонентам (`Click`, `Change`) и app lifecycle (`Load`);
- event handler получает значения выбранных input refs и обновляет выбранные output refs;
- существующие media/state/queue/cancel механизмы переиспользуются;
- сложные Gradio-возможности (`Tabs`, `Accordion`, chaining, batching, auth, share, durable sessions) не входят в первый цикл.

## Публичный API

Пример целевого API:

```go
app.Blocks(func(b *goleo.Blocks) {
	prompt := b.Textbox("Prompt")
	style := b.Dropdown("Style", "concise", "detailed")
	run := b.Button("Run")
	out := b.Textbox("Result")

	run.Click(
		goleo.Handler(func(prompt string, style string) (string, error) {
			return prompt + " / " + style, nil
		}),
		goleo.Inputs(prompt, style),
		goleo.Outputs(out),
	)

	prompt.Change(
		goleo.Handler(func(prompt string) (goleo.Update, error) {
			return goleo.Disabled(prompt == ""), nil
		}),
		goleo.Inputs(prompt),
		goleo.Outputs(run),
	)
})
```

Новые публичные типы:

- `goleo.Blocks`: builder для low-level UI.
- `goleo.ComponentRef`: ссылка на компонент с сохранением type/id/schema.
- `goleo.Update`: частичное обновление компонента.

`Update` MVP-поля:

```go
type Update struct {
	Value    any
	Visible  *bool
	Disabled *bool
	Choices  []string
	Label    *string
}
```

Для удобства добавляются helpers:

- `goleo.Visible(bool)`
- `goleo.Disabled(bool)`
- `goleo.Choices(...string)`
- `goleo.Label(string)`
- `goleo.Value(any)`

Backend определяет `goleo.Update` по Go-типу и отдает во frontend как update envelope:

```json
{
  "kind": "update",
  "value": "...",
  "disabled": true
}
```

Это нужно, чтобы обычный JSON output с полями `value` или `disabled` не был ошибочно принят за обновление компонента.

## Schema

Добавляется новый interface kind:

```json
{
  "id": "blocks-1",
  "kind": "blocks",
  "inputs": [],
  "outputs": [],
  "components": [],
  "events": []
}
```

`components` содержит дерево/список уже существующих `component.Component`.

`events` содержит:

```json
{
  "id": "blocks-1-event-1",
  "trigger": "click",
  "source": "blocks-1-component-3",
  "inputs": ["blocks-1-component-1", "blocks-1-component-2"],
  "outputs": ["blocks-1-component-4"]
}
```

Поддерживаемые triggers в MVP:

- `click`
- `change`
- `load`

## Backend Flow

Добавляется route:

```http
POST /api/event
```

Request:

```json
{
  "interface_id": "blocks-1",
  "event_id": "blocks-1-event-1",
  "data": {
    "component-id": "value"
  }
}
```

Response:

```json
{
  "data": {
    "component-id": "value-or-update-envelope"
  }
}
```

Backend выполняет:

1. найти blocks interface и event binding;
2. собрать positional input data по `event.inputs`;
3. выполнить state merge для `State` refs;
4. выполнить media hydration для input refs;
5. вызвать `runtime.HandlerBinding.Invoke`;
6. выполнить media dehydration для output refs;
7. применить `State` outputs;
8. вернуть map по output component ids.

Queue/cancel:

- `/api/event` использует тот же `queueManager`, что `/api/predict` и `/api/stream`;
- request id создается тем же middleware;
- `/api/cancel` должен уметь отменять event request по `X-Request-ID`.

## Frontend Flow

Frontend для `kind: "blocks"`:

- рендерит `components` как обычную форму без глобального submit;
- хранит values по component id;
- на `click` отправляет событие сразу;
- на `change` отправляет событие после изменения значения;
- `load` отправляется один раз после mount;
- применяет response по output ids.

`Update` применение:

- если response value имеет поля update payload, обновить component props/value;
- `value` обновляет текущий value;
- `visible` влияет на отображение;
- `disabled` влияет на control disabled;
- `choices` обновляет dropdown options;
- `label` обновляет отображаемый label.

Для MVP change-события можно отправлять без debounce. Если тесты покажут шумные вызовы, добавить простой debounce 150-250ms.

## Ограничения MVP

Не реализуется в первом цикле:

- event chaining (`then`, `success`, `failure`);
- streaming events for Blocks;
- batch events;
- Tabs/Accordion;
- JS/Python client parity;
- multi-user durable state;
- component update patches для произвольных props кроме перечисленных в `Update`.

## Тестирование

Backend unit/integration:

- schema включает `kind=blocks`, components и events;
- `/api/event` вызывает handler с positional inputs;
- output values возвращаются map по output ids;
- `Update` сериализуется и применяется как response value;
- media inputs/outputs внутри event переиспользуют существующий hydrate/dehydrate путь;
- queue full возвращает `429 queue_full`;
- cancel отменяет event handler context.

Frontend tests:

- blocks schema рендерится без submit-кнопки формы;
- click event вызывает `/api/event`;
- change event вызывает `/api/event`;
- output value обновляет компонент;
- update payload меняет disabled/visible/choices/label;
- load event запускается один раз.

Docs/examples:

- добавить `examples/blocks`;
- обновить README с коротким разделом `Interface vs Blocks`;
- обновить `docs/usage.md`, `docs/components.md`, `docs/architecture.md`.

## Совместимость

Существующие `Interface`, `Chat`, `Voice`, `/api/predict`, `/api/stream`, `/api/upload`, `/api/assets/{id}` не меняют wire contract.

Blocks добавляется как новая поверхность, поэтому backward compatibility сохраняется.
