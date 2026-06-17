package main

import (
	"context"
	"fmt"
	"log/slog"
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

	app.Interface(
		goleo.Handler(func(clip goleo.AudioInput, task string, normalized bool) (string, map[string]any, goleo.AudioOutput, error) {
			if clip.Path == "" {
				return "", nil, goleo.AudioOutput{}, fmt.Errorf("prompt audio is required")
			}

			name := clip.Name
			if name == "" {
				name = filepath.Base(clip.Path)
			}

			ext := filepath.Ext(name)
			base := strings.TrimSuffix(name, ext)
			if base == "" {
				base = "recording"
			}
			if ext == "" {
				ext = ".webm"
			}

			summary := fmt.Sprintf(
				"Audio turn complete.\n\nTask: %s\nFile: %s\nContent type: %s\nSize: %d bytes\nNormalized: %t\nAsset ID: %s",
				task,
				name,
				clip.ContentType,
				clip.Size,
				normalized,
				clip.ID,
			)

			metadata := map[string]any{
				"task":         task,
				"asset_id":     clip.ID,
				"name":         name,
				"size":         clip.Size,
				"content_type": clip.ContentType,
				"normalized":   normalized,
				"source_url":   clip.URL,
			}

			reply := goleo.AudioOutput{
				Name:        base + "-preview" + ext,
				ContentType: clip.ContentType,
				Path:        clip.Path,
			}

			return summary, metadata, reply, nil
		}),
		goleo.Inputs(
			goleo.Audio("Prompt audio", goleo.WithAccept("audio/*")),
			goleo.Dropdown("Task", "Preview for review", "Archive intake", "QA handoff"),
			goleo.Checkbox("Mark as normalized", goleo.WithDefault(true)),
		),
		goleo.Outputs(
			goleo.Textbox("Turn summary", goleo.WithRows(8)),
			goleo.JSON("Detected metadata"),
			goleo.Audio("Reply audio"),
		),
	)

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
