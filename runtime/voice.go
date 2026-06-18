package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"

	"github.com/sneiko/goleo/media"
)

var voiceSessionType = reflect.TypeOf((*VoiceSession)(nil))

// VoiceAudio carries one incoming audio chunk from the browser.
type VoiceAudio struct {
	MimeType string `json:"mime_type,omitempty"`
	Sequence int    `json:"sequence,omitempty"`
	Data     []byte `json:"data,omitempty"`
}

// VoiceEvent is the public session event contract for duplex voice flows.
type VoiceEvent struct {
	Type     string          `json:"type"`
	Status   string          `json:"status,omitempty"`
	Text     string          `json:"text,omitempty"`
	State    string          `json:"state,omitempty"`
	Progress json.RawMessage `json:"progress,omitempty"`
	Audio    *VoiceAudio     `json:"audio,omitempty"`
}

// VoiceBinding is the executable backend bound to a voice session surface.
type VoiceBinding struct {
	handle func(context.Context, *VoiceSession) error
}

type VoiceOutbound struct {
	Event       *VoiceEvent
	AudioOutput *media.AudioOutput
}

// VoiceSession provides bidirectional event exchange for a voice connection.
type VoiceSession struct {
	ctx      context.Context
	incoming <-chan VoiceEvent
	outgoing chan<- VoiceOutbound
}

// VoiceHandler wraps a duplex session function for use by App.Voice.
func VoiceHandler(fn any) *VoiceBinding {
	return &VoiceBinding{handle: makeVoiceHandle(fn)}
}

func (binding *VoiceBinding) Run(ctx context.Context, session *VoiceSession) (err error) {
	defer recoverHandlerPanic(&err)

	if binding == nil || binding.handle == nil {
		return errors.New("voice handler is not callable")
	}

	return binding.handle(ctx, session)
}

func NewVoiceSession(ctx context.Context, incoming <-chan VoiceEvent, outgoing chan<- VoiceOutbound) *VoiceSession {
	return &VoiceSession{
		ctx:      ctx,
		incoming: incoming,
		outgoing: outgoing,
	}
}

func (session *VoiceSession) Context() context.Context {
	if session == nil || session.ctx == nil {
		return context.Background()
	}

	return session.ctx
}

func (session *VoiceSession) Receive() (VoiceEvent, error) {
	if session == nil {
		return VoiceEvent{}, errors.New("voice session is nil")
	}

	select {
	case <-session.Context().Done():
		return VoiceEvent{}, session.Context().Err()
	case event, ok := <-session.incoming:
		if !ok {
			return VoiceEvent{}, io.EOF
		}
		return event, nil
	}
}

func (session *VoiceSession) Send(event VoiceEvent) error {
	return session.send(VoiceOutbound{Event: &event})
}

func (session *VoiceSession) SendAudio(output media.AudioOutput) error {
	return session.send(VoiceOutbound{AudioOutput: &output})
}

func (session *VoiceSession) send(outbound VoiceOutbound) error {
	if session == nil {
		return errors.New("voice session is nil")
	}

	select {
	case <-session.Context().Done():
		return session.Context().Err()
	case session.outgoing <- outbound:
		return nil
	}
}

func makeVoiceHandle(fn any) func(context.Context, *VoiceSession) error {
	fnValue := reflect.ValueOf(fn)
	if !fnValue.IsValid() {
		return func(context.Context, *VoiceSession) error {
			return errors.New("voice handler must be a function")
		}
	}

	fnType := fnValue.Type()
	if fnType.Kind() != reflect.Func {
		return func(context.Context, *VoiceSession) error {
			return errors.New("voice handler must be a function")
		}
	}

	return func(ctx context.Context, session *VoiceSession) error {
		args := make([]reflect.Value, 0, fnType.NumIn())
		switch fnType.NumIn() {
		case 1:
			if fnType.In(0) != voiceSessionType {
				return fmt.Errorf("voice handler must accept *runtime.VoiceSession")
			}
			args = append(args, reflect.ValueOf(session))
		case 2:
			if !fnType.In(0).Implements(contextType) || fnType.In(1) != voiceSessionType {
				return fmt.Errorf("voice handler must accept (context.Context, *runtime.VoiceSession)")
			}
			args = append(args, reflect.ValueOf(ctx), reflect.ValueOf(session))
		default:
			return fmt.Errorf("voice handler must accept 1 or 2 arguments")
		}

		results := fnValue.Call(args)
		_, err := unpackResults(results)
		return err
	}
}
