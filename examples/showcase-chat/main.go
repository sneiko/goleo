package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sneiko/goleo"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	app := goleo.New(goleo.WithLogger(logger))

	app.Chat(goleo.StreamHandler(func(input string, emit goleo.EmitFunc) error {
		chunks := []string{
			"Start with the outcome your team cares about.\n\n",
			fmt.Sprintf("For this request, frame the message around: %s\n\n", input),
			"Then close with one rollout detail, one success metric, and one next action.",
		}

		for _, chunk := range chunks {
			emit(chunk)
			time.Sleep(20 * time.Millisecond)
		}
		return nil
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
