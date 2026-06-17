package goleo

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/sneiko/goleo/core"
	"github.com/sneiko/goleo/server"
)

// App is a runnable AI demo application.
type App struct {
	*core.App
}

// Option customizes an app during construction.
type Option func(*App)

// LaunchOptions configures the built-in HTTP server.
type LaunchOptions struct {
	Addr              string
	ReadTimeout       time.Duration
	ReadHeaderTimeout time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
}

// WithLogger configures structured logging for the app.
func WithLogger(logger *slog.Logger) Option {
	return func(app *App) {
		app.SetLogger(logger)
	}
}

// New creates an empty Goleo app.
func New(options ...Option) *App {
	app := &App{
		App: core.New(),
	}

	for _, option := range options {
		option(app)
	}

	return app
}

// Handler returns the complete HTTP handler for the app.
func (app *App) Handler() http.Handler {
	return server.New(app.App)
}

// Server returns a configured net/http server for the app.
func (app *App) Server(options LaunchOptions) *http.Server {
	addr := options.Addr
	if addr == "" {
		addr = ":7860"
	}

	return &http.Server{
		Addr:              addr,
		Handler:           app.Handler(),
		ReadTimeout:       options.ReadTimeout,
		ReadHeaderTimeout: options.ReadHeaderTimeout,
		WriteTimeout:      options.WriteTimeout,
		IdleTimeout:       options.IdleTimeout,
	}
}

// Launch starts the app with net/http.
func (app *App) Launch(options LaunchOptions) error {
	server := app.Server(options)
	app.Logger().Info("starting goleo server", "addr", server.Addr)

	return server.ListenAndServe()
}

// LaunchContext starts the app and gracefully shuts it down when ctx is canceled.
func (app *App) LaunchContext(ctx context.Context, options LaunchOptions) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	server := app.Server(options)
	logger := app.Logger()
	logger.Info("starting goleo server", "addr", server.Addr)

	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()

		shutdownTimeout := options.ShutdownTimeout
		if shutdownTimeout == 0 {
			shutdownTimeout = 5 * time.Second
		}

		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		logger.Info("shutting down goleo server", "addr", server.Addr, "timeout", shutdownTimeout.String())
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("goleo server shutdown failed", "error", err)
		}
	}()

	err := server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		<-shutdownDone
		return nil
	}

	return err
}
