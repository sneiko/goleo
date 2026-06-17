package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/sneiko/goleo"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	app := goleo.New(goleo.WithLogger(logger))

	app.Voice(goleo.VoiceHandler(func(ctx context.Context, session *goleo.VoiceSession) error {
		tempDir, err := os.MkdirTemp("", "goleo-voice-example-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tempDir)

		var (
			captureFile *os.File
			capturePath string
			captureMime string
			chunkCount  int
			byteCount   int
			turn        int
		)

		closeCapture := func() error {
			if captureFile == nil {
				return nil
			}

			err := captureFile.Close()
			captureFile = nil
			return err
		}

		resetCapture := func() {
			capturePath = ""
			captureMime = ""
			chunkCount = 0
			byteCount = 0
		}

		for {
			event, err := session.Receive()
			if err != nil {
				if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
					return nil
				}
				return err
			}

			switch event.Type {
			case "session.start":
				if err := session.Send(goleo.VoiceEvent{Type: "session.ready"}); err != nil {
					return err
				}
				if err := session.Send(goleo.VoiceEvent{Type: "output.state", State: "listening"}); err != nil {
					return err
				}
			case "input.audio":
				if event.Audio == nil || len(event.Audio.Data) == 0 {
					continue
				}

				if captureFile == nil {
					turn++
					captureMime = normalizeMimeType(event.Audio.MimeType)
					capturePath = filepath.Join(tempDir, fmt.Sprintf("turn-%02d%s", turn, extensionForAudioType(captureMime)))
					captureFile, err = os.OpenFile(capturePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
					if err != nil {
						return err
					}
					if err := session.Send(goleo.VoiceEvent{Type: "output.state", State: "capturing"}); err != nil {
						return err
					}
				}

				if _, err := captureFile.Write(event.Audio.Data); err != nil {
					return err
				}
				chunkCount++
				byteCount += len(event.Audio.Data)
			case "input.stop":
				if captureFile == nil {
					if err := session.Send(goleo.VoiceEvent{Type: "error", Text: "received input.stop before any audio chunks"}); err != nil {
						return err
					}
					continue
				}

				if err := closeCapture(); err != nil {
					return err
				}

				if err := session.Send(goleo.VoiceEvent{
					Type: "output.text",
					Text: fmt.Sprintf(
						"Turn %d complete. Captured %d chunks and %d bytes of %s audio.",
						turn,
						chunkCount,
						byteCount,
						captureMime,
					),
				}); err != nil {
					return err
				}

				if err := session.SendAudio(goleo.AudioOutput{
					Name:        filepath.Base(capturePath),
					ContentType: captureMime,
					Path:        capturePath,
				}); err != nil {
					return err
				}

				if err := session.Send(goleo.VoiceEvent{Type: "output.state", State: "idle"}); err != nil {
					return err
				}

				resetCapture()
			case "output.interrupt":
				if err := session.Send(goleo.VoiceEvent{Type: "output.state", State: "interrupted"}); err != nil {
					return err
				}
			case "session.close":
				_ = closeCapture()
				if err := session.Send(goleo.VoiceEvent{Type: "session.closed"}); err != nil {
					return err
				}
				return nil
			}
		}
	}))

	addr := os.Getenv("GOLEO_ADDR")
	if addr == "" {
		addr = ":7860"
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.LaunchContext(ctx, goleo.LaunchOptions{Addr: addr}); err != nil {
		logger.Error("goleo server stopped", "error", err)
		os.Exit(1)
	}
}

func normalizeMimeType(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "audio/webm"
	}

	return value
}

func extensionForAudioType(contentType string) string {
	extensions, err := mime.ExtensionsByType(contentType)
	if err == nil && len(extensions) > 0 {
		return extensions[0]
	}

	switch contentType {
	case "audio/mpeg", "audio/mp3":
		return ".mp3"
	case "audio/wav":
		return ".wav"
	default:
		return ".webm"
	}
}
