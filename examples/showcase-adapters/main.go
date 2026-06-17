package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/sneiko/goleo"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	app := goleo.New(goleo.WithLogger(logger))

	app.Interface(
		goleo.Handler(func(prompt string, backend string, streaming bool) (string, map[string]any, error) {
			if backend == "" {
				backend = "HTTP adapter"
			}

			result := fmt.Sprintf(
				"Backend profile: %s\nStreaming UX: %t\n\nNormalized result:\n%s",
				backend,
				streaming,
				prompt,
			)

			payload := map[string]any{
				"backend":    backend,
				"streaming":  streaming,
				"transport":  "normalized handler binding",
				"input_type": "single prompt",
				"result":     "stable local demo output",
			}

			return result, payload, nil
		}),
		goleo.Inputs(
			goleo.Textbox(
				"Prompt",
				goleo.WithRows(4),
				goleo.WithDefault("Summarize the internal support copilot launch for team leads."),
				goleo.WithPlaceholder("Describe the backend task you want to wrap."),
			),
			goleo.CustomComponent(
				"dropdown",
				"Backend profile",
				goleo.WithChoices("HTTP adapter", "OpenAI-compatible", "Ollama"),
				goleo.WithDefault("HTTP adapter"),
			),
			goleo.Checkbox("Streaming UX", goleo.WithDefault(true)),
		),
		goleo.Outputs(
			goleo.Textbox(
				"Normalized result",
				goleo.WithRows(8),
				goleo.WithDefault("Backend profile: HTTP adapter\nStreaming UX: true\n\nNormalized result:\nSummarize the internal support copilot launch for team leads."),
			),
			goleo.JSON(
				"Backend metadata",
				goleo.WithDefault(map[string]any{
					"backend":    "HTTP adapter",
					"streaming":  true,
					"transport":  "normalized handler binding",
					"input_type": "single prompt",
					"result":     "stable local demo output",
				}),
			),
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
