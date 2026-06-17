package goleo

import "github.com/sneiko/goleo/runtime"

type EmitFunc = runtime.EmitFunc
type HandlerBinding = runtime.HandlerBinding

func Handler(fn any) *HandlerBinding {
	return runtime.Handler(fn)
}

func StreamHandler(fn any) *HandlerBinding {
	return runtime.StreamHandler(fn)
}
