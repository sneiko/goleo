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
