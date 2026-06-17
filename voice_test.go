package goleo_test

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/sneiko/goleo"
)

func TestVoiceSchemaRegistersVoiceInterfaceKind(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	app.Voice(goleo.VoiceHandler(func(session *goleo.VoiceSession) error {
		return nil
	}))

	schema := app.Schema()
	if len(schema.Interfaces) != 1 {
		t.Fatalf("len(schema.Interfaces) = %d, want 1", len(schema.Interfaces))
	}
	if got := schema.Interfaces[0].Kind; got != "voice" {
		t.Fatalf("kind = %q, want voice", got)
	}
}

func TestVoiceWebSocketSessionExchangesTextAndAudioEvents(t *testing.T) {
	t.Parallel()

	outputDir := t.TempDir()
	app := goleo.New()
	app.Voice(goleo.VoiceHandler(func(session *goleo.VoiceSession) error {
		for {
			event, err := session.Receive()
			if err != nil {
				return err
			}

			switch event.Type {
			case "session.start":
				if err := session.Send(goleo.VoiceEvent{Type: "session.ready"}); err != nil {
					return err
				}
			case "input.audio":
				outputPath := filepath.Join(outputDir, "reply.wav")
				if err := os.WriteFile(outputPath, bytes.ToUpper(event.Audio.Data), 0o600); err != nil {
					return err
				}
				if err := session.Send(goleo.VoiceEvent{Type: "output.text", Text: "heard hello"}); err != nil {
					return err
				}
				if err := session.SendAudio(goleo.AudioOutput{
					Name:        "reply.wav",
					ContentType: "audio/wav",
					Path:        outputPath,
				}); err != nil {
					return err
				}
			case "session.close":
				_ = session.Send(goleo.VoiceEvent{Type: "session.closed"})
				return nil
			}
		}
	}))

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/voice/voice-1/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{"type": "session.start"}); err != nil {
		t.Fatalf("write session.start: %v", err)
	}

	var ready struct {
		Type string `json:"type"`
	}
	if err := conn.ReadJSON(&ready); err != nil {
		t.Fatalf("read ready: %v", err)
	}
	if ready.Type != "session.ready" {
		t.Fatalf("ready type = %q, want session.ready", ready.Type)
	}

	if err := conn.WriteJSON(map[string]any{
		"type": "input.audio",
		"audio": map[string]any{
			"mime_type": "audio/wav",
			"sequence":  1,
			"data":      base64.StdEncoding.EncodeToString([]byte("hello")),
		},
	}); err != nil {
		t.Fatalf("write input.audio: %v", err)
	}

	var textEvent struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := conn.ReadJSON(&textEvent); err != nil {
		t.Fatalf("read text event: %v", err)
	}
	if textEvent.Type != "output.text" || textEvent.Text != "heard hello" {
		t.Fatalf("text event = %#v, want output.text/heard hello", textEvent)
	}

	var audioEvent struct {
		Type  string           `json:"type"`
		Audio goleo.AudioAsset `json:"audio"`
	}
	if err := conn.ReadJSON(&audioEvent); err != nil {
		t.Fatalf("read audio event: %v", err)
	}
	if audioEvent.Type != "output.audio" {
		t.Fatalf("audio event type = %q, want output.audio", audioEvent.Type)
	}
	if audioEvent.Audio.URL == "" {
		t.Fatalf("audio event = %#v, want asset url", audioEvent)
	}

	assetResp, err := http.Get(server.URL + audioEvent.Audio.URL)
	if err != nil {
		t.Fatalf("get audio asset: %v", err)
	}
	defer assetResp.Body.Close()

	var assetBody bytes.Buffer
	if _, err := assetBody.ReadFrom(assetResp.Body); err != nil {
		t.Fatalf("read asset body: %v", err)
	}
	if got := assetBody.String(); got != "HELLO" {
		t.Fatalf("asset body = %q, want HELLO", got)
	}

	if err := conn.WriteJSON(map[string]any{"type": "session.close"}); err != nil {
		t.Fatalf("write session.close: %v", err)
	}

	var closed struct {
		Type string `json:"type"`
	}
	if err := conn.ReadJSON(&closed); err != nil {
		t.Fatalf("read closed event: %v", err)
	}
	if closed.Type != "session.closed" {
		t.Fatalf("closed type = %q, want session.closed", closed.Type)
	}
}

func TestVoiceWebSocketSessionInterruptsOutput(t *testing.T) {
	t.Parallel()

	app := goleo.New()
	app.Voice(goleo.VoiceHandler(func(session *goleo.VoiceSession) error {
		for {
			event, err := session.Receive()
			if err != nil {
				return err
			}

			switch event.Type {
			case "output.interrupt":
				return session.Send(goleo.VoiceEvent{Type: "output.state", State: "interrupted"})
			case "session.close":
				return nil
			}
		}
	}))

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/voice/voice-1/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{"type": "output.interrupt"}); err != nil {
		t.Fatalf("write output.interrupt: %v", err)
	}

	var stateEvent struct {
		Type  string `json:"type"`
		State string `json:"state"`
	}
	if err := conn.ReadJSON(&stateEvent); err != nil {
		t.Fatalf("read state event: %v", err)
	}
	if stateEvent.Type != "output.state" || stateEvent.State != "interrupted" {
		t.Fatalf("state event = %#v, want interrupted state", stateEvent)
	}
}
