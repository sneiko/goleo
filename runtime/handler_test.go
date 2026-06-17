package runtime

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestInvokeCallsHandlerWithConvertedInputs(t *testing.T) {
	t.Parallel()

	binding := Handler(func(name string, count int) (string, error) {
		return strings.Repeat(name, count), nil
	})

	got, err := binding.Invoke(context.Background(), []any{"go", float64(2)})
	if err != nil {
		t.Fatalf("Invoke error = %v", err)
	}
	if !reflect.DeepEqual(got, []any{"gogo"}) {
		t.Fatalf("Invoke result = %#v, want [gogo]", got)
	}
}

func TestInvokeInjectsContext(t *testing.T) {
	t.Parallel()

	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "request-value")
	binding := Handler(func(ctx context.Context, input string) (string, error) {
		return ctx.Value(contextKey{}).(string) + ":" + input, nil
	})

	got, err := binding.Invoke(ctx, []any{"hello"})
	if err != nil {
		t.Fatalf("Invoke error = %v", err)
	}
	if !reflect.DeepEqual(got, []any{"request-value:hello"}) {
		t.Fatalf("Invoke result = %#v, want [request-value:hello]", got)
	}
}

func TestInvokeReturnsMultipleValues(t *testing.T) {
	t.Parallel()

	binding := Handler(func(input string) (string, int, error) {
		return input, len(input), nil
	})

	got, err := binding.Invoke(context.Background(), []any{"hello"})
	if err != nil {
		t.Fatalf("Invoke error = %v", err)
	}
	if !reflect.DeepEqual(got, []any{"hello", 5}) {
		t.Fatalf("Invoke result = %#v, want [hello 5]", got)
	}
}

func TestInvokeReturnsHandlerError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("handler failed")
	binding := Handler(func(string) (string, error) {
		return "", wantErr
	})

	got, err := binding.Invoke(context.Background(), []any{"hello"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Invoke error = %v, want %v", err, wantErr)
	}
	if got != nil {
		t.Fatalf("Invoke result = %#v, want nil", got)
	}
}

func TestInvokeRejectsInvalidInputCounts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data []any
		want string
	}{
		{name: "missing", data: []any{}, want: "missing input 0"},
		{name: "extra", data: []any{"hello", "extra"}, want: "got 2 inputs, handler accepts 1"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			binding := Handler(func(string) string { return "ok" })
			_, err := binding.Invoke(context.Background(), test.data)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Invoke error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestInvokeRejectsInvalidFunction(t *testing.T) {
	t.Parallel()

	binding := Handler("not a function")
	_, err := binding.Invoke(context.Background(), nil)
	if err == nil || err.Error() != "handler must be a function" {
		t.Fatalf("Invoke error = %v, want handler must be a function", err)
	}
}

func TestStreamEmitsValues(t *testing.T) {
	t.Parallel()

	binding := StreamHandler(func(ctx context.Context, input string, emit EmitFunc) error {
		emit(ctx.Err())
		emit(input + " one")
		emit(input + " two")
		return nil
	})

	var got []any
	err := binding.Stream(context.Background(), []any{"go"}, func(value any) {
		got = append(got, value)
	})
	if err != nil {
		t.Fatalf("Stream error = %v", err)
	}
	if !reflect.DeepEqual(got, []any{nil, "go one", "go two"}) {
		t.Fatalf("emitted = %#v, want [<nil> go one go two]", got)
	}
}

func TestStreamReturnsHandlerError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("stream failed")
	binding := StreamHandler(func(string, EmitFunc) error {
		return wantErr
	})

	err := binding.Stream(context.Background(), []any{"hello"}, func(any) {})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Stream error = %v, want %v", err, wantErr)
	}
}

func TestStreamRejectsInvalidSignature(t *testing.T) {
	t.Parallel()

	binding := StreamHandler(func(string) error { return nil })
	err := binding.Stream(context.Background(), []any{"hello"}, func(any) {})
	if err == nil || err.Error() != "stream handler must accept goleo.EmitFunc as the last argument" {
		t.Fatalf("Stream error = %v, want invalid signature error", err)
	}
}

func TestInvokeRecoversHandlerPanic(t *testing.T) {
	t.Parallel()

	binding := Handler(func(string) string {
		panic("boom")
	})

	_, err := binding.Invoke(context.Background(), []any{"hello"})
	if err == nil {
		t.Fatal("Invoke error is nil, want panic error")
	}

	var panicErr PanicError
	if !errors.As(err, &panicErr) {
		t.Fatalf("Invoke error = %T %[1]v, want PanicError", err)
	}
	if panicErr.Value != "boom" {
		t.Fatalf("panic value = %#v, want boom", panicErr.Value)
	}
}

func TestStreamRecoversHandlerPanic(t *testing.T) {
	t.Parallel()

	binding := StreamHandler(func(string, EmitFunc) error {
		panic("stream boom")
	})

	err := binding.Stream(context.Background(), []any{"hello"}, func(any) {})
	if err == nil {
		t.Fatal("Stream error is nil, want panic error")
	}

	var panicErr PanicError
	if !errors.As(err, &panicErr) {
		t.Fatalf("Stream error = %T %[1]v, want PanicError", err)
	}
	if panicErr.Value != "stream boom" {
		t.Fatalf("panic value = %#v, want stream boom", panicErr.Value)
	}
}

func TestCallableRecoversHandlerPanic(t *testing.T) {
	t.Parallel()

	binding := Callable(func(context.Context, []any) ([]any, error) {
		panic("callable boom")
	})

	_, err := binding.Invoke(context.Background(), nil)
	if err == nil {
		t.Fatal("Invoke error is nil, want panic error")
	}

	var panicErr PanicError
	if !errors.As(err, &panicErr) {
		t.Fatalf("Invoke error = %T %[1]v, want PanicError", err)
	}
	if panicErr.Value != "callable boom" {
		t.Fatalf("panic value = %#v, want callable boom", panicErr.Value)
	}
}
