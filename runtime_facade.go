package goleo

import "github.com/sneiko/goleo/runtime"

type EmitFunc = runtime.EmitFunc
type HandlerBinding = runtime.HandlerBinding
type VoiceBinding = runtime.VoiceBinding
type VoiceSession = runtime.VoiceSession
type VoiceEvent = runtime.VoiceEvent
type VoiceAudio = runtime.VoiceAudio

func Handler(fn any) *HandlerBinding {
	return runtime.Handler(fn)
}

func StreamHandler(fn any) *HandlerBinding {
	return runtime.StreamHandler(fn)
}

func VoiceHandler(fn any) *VoiceBinding {
	return runtime.VoiceHandler(fn)
}
