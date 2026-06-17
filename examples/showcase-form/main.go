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

type uploadedFile struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	app := goleo.New(goleo.WithLogger(logger))

	app.Interface(
		goleo.Handler(func(
			topic string,
			maxWords int,
			temperature float64,
			includeCTA bool,
			audience string,
			brief uploadedFile,
		) (string, map[string]any, error) {
			if audience == "" {
				audience = "Product leaders"
			}

			fileName := brief.Name
			if fileName == "" {
				fileName = "support-brief.md"
			}

			summary := fmt.Sprintf(
				"Launch summary\n\nTopic: %s\nAudience: %s\nWord budget: %d\nTemperature: %.1f\nCTA included: %t\nReference file: %s",
				topic,
				audience,
				maxWords,
				temperature,
				includeCTA,
				fileName,
			)

			payload := map[string]any{
				"topic":          topic,
				"audience":       audience,
				"max_words":      maxWords,
				"temperature":    temperature,
				"include_cta":    includeCTA,
				"headline":       "Internal support copilot rollout",
				"success_metric": "Reduce repetitive ticket handling time",
				"reference_file": map[string]any{
					"name":         fileName,
					"size":         brief.Size,
					"content_type": brief.ContentType,
				},
			}

			return summary, payload, nil
		}),
		goleo.Inputs(
			goleo.Textbox(
				"Topic",
				goleo.WithPlaceholder("Describe the launch, workflow, or tool you want to explain."),
				goleo.WithDefault("Launch an internal support copilot for customer operations."),
				goleo.WithRows(4),
			),
			goleo.Number("Max words", goleo.WithMin(80), goleo.WithMax(280), goleo.WithStep(10), goleo.WithDefault(140)),
			goleo.Slider("Temperature", goleo.WithMin(0), goleo.WithMax(1), goleo.WithStep(0.1), goleo.WithDefault(0.6)),
			goleo.Checkbox("Include call to action", goleo.WithDefault(true)),
			goleo.CustomComponent(
				"dropdown",
				"Audience",
				goleo.WithChoices("Product leaders", "Support teams", "Developers"),
				goleo.WithDefault("Product leaders"),
			),
			goleo.File(
				"Reference brief",
				goleo.WithAccept(".txt,.md,.pdf"),
				goleo.WithDefault(uploadedFile{
					ID:          "upload-support-brief",
					Name:        "support-brief.md",
					Size:        1842,
					ContentType: "text/markdown",
				}),
			),
		),
		goleo.Outputs(
			goleo.Textbox(
				"Launch summary",
				goleo.WithRows(8),
				goleo.WithDefault("Launch summary\n\nTopic: Launch an internal support copilot for customer operations.\nAudience: Product leaders\nWord budget: 140\nTemperature: 0.6\nCTA included: true\nReference file: support-brief.md"),
			),
			goleo.JSON(
				"Structured output",
				goleo.WithDefault(map[string]any{
					"topic":          "Launch an internal support copilot for customer operations.",
					"audience":       "Product leaders",
					"max_words":      140,
					"temperature":    0.6,
					"include_cta":    true,
					"headline":       "Internal support copilot rollout",
					"success_metric": "Reduce repetitive ticket handling time",
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
