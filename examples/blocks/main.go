package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/sneiko/goleo"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	app := goleo.New(goleo.WithLogger(logger))

	app.Blocks(func(blocks *goleo.Blocks) {
		prompt := blocks.Textbox(
			"Prompt",
			goleo.WithDefault("Summarize the latest product planning notes."),
			goleo.WithPlaceholder("Type a request for the assistant"),
			goleo.WithRows(5),
		)
		style := blocks.Dropdown("Style", "concise", "friendly", "technical")
		includeMetadata := blocks.Checkbox("Include metadata", goleo.WithDefault(true))
		generate := blocks.Button("Generate")
		reset := blocks.Button("Reset run count")
		result := blocks.Textbox("Draft", goleo.WithRows(6))
		meta := blocks.JSON("Run metadata")
		counter := blocks.State("Run count", goleo.WithDefault(0), goleo.WithVisible(false))

		prompt.Change(
			goleo.Handler(func(promptText string) (string, error) {
				return fmt.Sprintf("Preview: %s", strings.TrimSpace(promptText)), nil
			}),
			goleo.Inputs(prompt),
			goleo.Outputs(result),
		)

		blocks.Load(
			goleo.Handler(func() (string, goleo.Update, error) {
				return "Type a prompt and click Generate.", goleo.Disabled(true), nil
			}),
			goleo.Outputs(result, reset),
		)

		generate.Click(
			goleo.Handler(func(promptText, style string, includeMeta bool, runCount int) (string, map[string]any, goleo.Update, int, error) {
				clean := strings.TrimSpace(promptText)
				if clean == "" {
					return "Prompt is empty, please enter text first.", nil, goleo.Disabled(true), runCount, nil
				}

				runCount++
				summary := fmt.Sprintf("[%s] %s", style, clean)
				if style == "concise" {
					summary = summary[:minInt(160, len(summary))]
				}

				metadata := map[string]any{
					"prompt_length": len(clean),
					"style":         style,
					"include_meta":  includeMeta,
					"run_count":     runCount,
					"generated_at":  time.Now().Format(time.RFC3339),
				}
				if includeMeta {
					metadata["preview"] = summary
				}

				return summary, metadata, goleo.Disabled(false), runCount, nil
			}),
			goleo.Inputs(prompt, style, includeMetadata, counter),
			goleo.Outputs(result, meta, reset, counter),
		)

		reset.Click(
			goleo.Handler(func(runCount int) (string, map[string]any, goleo.Update, int, error) {
				if runCount == 0 {
					return "Run count already at zero.", map[string]any{}, goleo.Disabled(true), 0, nil
				}

				metadata := map[string]any{
					"message":      "run counter reset",
					"run_count":    0,
					"generated_at": time.Now().Format(time.RFC3339),
				}
				return "History reset. Start a new run.", metadata, goleo.Disabled(true), 0, nil
			}),
			goleo.Inputs(counter),
			goleo.Outputs(result, meta, reset, counter),
		)
	})

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

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
