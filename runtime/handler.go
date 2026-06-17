package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
)

var (
	contextType = reflect.TypeOf((*context.Context)(nil)).Elem()
	errorType   = reflect.TypeOf((*error)(nil)).Elem()
	emitType    = reflect.TypeOf((EmitFunc)(nil))
)

// EmitFunc sends one streaming response chunk.
type EmitFunc func(value any)

// HandlerBinding is the executable backend bound to a UI interface.
type HandlerBinding struct {
	call   func(context.Context, []any) ([]any, error)
	stream func(context.Context, []any, EmitFunc) error
}

// PanicError is returned when a user-provided handler panics.
type PanicError struct {
	Value any
}

func (err PanicError) Error() string {
	return "handler panic"
}

// Handler wraps a Go function for use by Interface.
func Handler(fn any) *HandlerBinding {
	return &HandlerBinding{
		call: makeReflectCall(fn),
	}
}

// StreamHandler wraps a Go function for use by Chat or /api/stream.
func StreamHandler(fn any) *HandlerBinding {
	return &HandlerBinding{
		stream: makeReflectStream(fn),
	}
}

// Callable creates a handler binding from a concrete callable.
func Callable(fn func(context.Context, []any) ([]any, error)) *HandlerBinding {
	return &HandlerBinding{
		call: fn,
	}
}

func (binding *HandlerBinding) Invoke(ctx context.Context, data []any) (values []any, err error) {
	defer recoverHandlerPanic(&err)

	if binding == nil || binding.call == nil {
		return nil, errors.New("handler is not callable")
	}

	return binding.call(ctx, data)
}

func (binding *HandlerBinding) Stream(ctx context.Context, data []any, emit EmitFunc) (err error) {
	defer recoverHandlerPanic(&err)

	if binding == nil || binding.stream == nil {
		return errors.New("handler is not streamable")
	}

	return binding.stream(ctx, data, emit)
}

func recoverHandlerPanic(err *error) {
	value := recover()
	if value == nil {
		return
	}

	*err = PanicError{Value: value}
}

func makeReflectCall(fn any) func(context.Context, []any) ([]any, error) {
	fnValue := reflect.ValueOf(fn)
	fnType := fnValue.Type()
	if fnType.Kind() != reflect.Func {
		return func(context.Context, []any) ([]any, error) {
			return nil, errors.New("handler must be a function")
		}
	}

	return func(ctx context.Context, data []any) ([]any, error) {
		args, err := buildArgs(ctx, data, fnType, false)
		if err != nil {
			return nil, err
		}

		results := fnValue.Call(args)
		return unpackResults(results)
	}
}

func makeReflectStream(fn any) func(context.Context, []any, EmitFunc) error {
	fnValue := reflect.ValueOf(fn)
	fnType := fnValue.Type()
	if fnType.Kind() != reflect.Func {
		return func(context.Context, []any, EmitFunc) error {
			return errors.New("stream handler must be a function")
		}
	}

	return func(ctx context.Context, data []any, emit EmitFunc) error {
		args, err := buildArgs(ctx, data, fnType, true)
		if err != nil {
			return err
		}
		args = append(args, reflect.ValueOf(emit))

		results := fnValue.Call(args)
		_, err = unpackResults(results)
		return err
	}
}

func buildArgs(ctx context.Context, data []any, fnType reflect.Type, hasEmit bool) ([]reflect.Value, error) {
	args := []reflect.Value{}
	paramIndex := 0
	dataIndex := 0
	limit := fnType.NumIn()
	if hasEmit {
		limit--
		if limit < 0 || fnType.In(fnType.NumIn()-1) != emitType {
			return nil, errors.New("stream handler must accept goleo.EmitFunc as the last argument")
		}
	}

	if limit > 0 && fnType.In(0).Implements(contextType) {
		args = append(args, reflect.ValueOf(ctx))
		paramIndex = 1
	}

	for ; paramIndex < limit; paramIndex++ {
		if dataIndex >= len(data) {
			return nil, fmt.Errorf("missing input %d", dataIndex)
		}

		value, err := convertValue(data[dataIndex], fnType.In(paramIndex))
		if err != nil {
			return nil, fmt.Errorf("input %d: %w", dataIndex, err)
		}
		args = append(args, value)
		dataIndex++
	}

	if dataIndex != len(data) {
		return nil, fmt.Errorf("got %d inputs, handler accepts %d", len(data), dataIndex)
	}

	return args, nil
}

func convertValue(input any, target reflect.Type) (reflect.Value, error) {
	if input == nil {
		return reflect.Zero(target), nil
	}

	inputValue := reflect.ValueOf(input)
	if inputValue.Type().AssignableTo(target) {
		return inputValue, nil
	}
	if inputValue.Type().ConvertibleTo(target) {
		return inputValue.Convert(target), nil
	}

	payload, err := json.Marshal(input)
	if err != nil {
		return reflect.Value{}, err
	}

	destination := reflect.New(target)
	if err := json.Unmarshal(payload, destination.Interface()); err != nil {
		return reflect.Value{}, err
	}

	return destination.Elem(), nil
}

func unpackResults(results []reflect.Value) ([]any, error) {
	values := []any{}
	for _, result := range results {
		if result.Type().Implements(errorType) {
			if result.IsNil() {
				continue
			}
			err, ok := result.Interface().(error)
			if !ok {
				return nil, errors.New("handler returned non-error value with error type")
			}
			return nil, err
		}
		values = append(values, result.Interface())
	}

	return values, nil
}
