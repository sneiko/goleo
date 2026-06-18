package server

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/sneiko/goleo/component"
	"github.com/sneiko/goleo/core"
	"github.com/sneiko/goleo/media"
	"github.com/sneiko/goleo/runtime"
)

type predictRequest struct {
	InterfaceID string `json:"interface_id"`
	Data        []any  `json:"data"`
}

type predictResponse struct {
	Data []any `json:"data"`
}

type eventRequest struct {
	InterfaceID string         `json:"interface_id"`
	EventID     string         `json:"event_id"`
	Data        map[string]any `json:"data"`
}

type eventResponse struct {
	Data map[string]any `json:"data"`
}

type uploadResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
	URL         string `json:"url,omitempty"`
}

type errorResponse struct {
	Error responseError `json:"error"`
}

type responseError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

const requestIDHeader = "X-Request-ID"

type requestIDContextKey struct{}

type requestRegistry struct {
	mu       sync.RWMutex
	cancels  map[string]context.CancelFunc
	requests map[string]struct{}
}

func newRequestRegistry() *requestRegistry {
	return &requestRegistry{
		cancels:  map[string]context.CancelFunc{},
		requests: map[string]struct{}{},
	}
}

func (registry *requestRegistry) register(id string, cancel context.CancelFunc) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	registry.requests[id] = struct{}{}
	registry.cancels[id] = cancel
}

func (registry *requestRegistry) unregister(id string) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	delete(registry.requests, id)
	delete(registry.cancels, id)
}

func (registry *requestRegistry) cancel(id string) bool {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	cancel, ok := registry.cancels[id]
	if !ok {
		return false
	}

	cancel()
	delete(registry.requests, id)
	delete(registry.cancels, id)
	return true
}

func (registry *requestRegistry) close() {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	for id, cancel := range registry.cancels {
		cancel()
		delete(registry.requests, id)
		delete(registry.cancels, id)
	}
}

type responseRecorder struct {
	http.ResponseWriter
	status int
}

type cancelRequest struct {
	RequestID string `json:"request_id"`
}

type cancelResponse struct {
	RequestID string `json:"request_id"`
	Cancelled bool   `json:"cancelled"`
}

type appHandler struct {
	next     http.Handler
	store    *assetStore
	requests *requestRegistry
}

func (recorder *responseRecorder) WriteHeader(status int) {
	if recorder.status != 0 {
		return
	}

	recorder.status = status
	recorder.ResponseWriter.WriteHeader(status)
}

func (recorder *responseRecorder) Flush() {
	flusher, ok := recorder.ResponseWriter.(http.Flusher)
	if !ok {
		return
	}
	if recorder.status == 0 {
		recorder.status = http.StatusOK
	}
	flusher.Flush()
}

func (recorder *responseRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := recorder.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("response writer does not implement http.Hijacker")
	}

	return hijacker.Hijack()
}

func (handler *appHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	handler.next.ServeHTTP(w, r)
}

func (handler *appHandler) Close() error {
	if handler == nil || handler.store == nil {
		return nil
	}
	if handler.requests != nil {
		handler.requests.close()
	}

	return handler.store.close()
}

// New creates the complete HTTP handler for an app.
func New(app *core.App) http.Handler {
	store, err := newAssetStore()
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		})
	}
	maxConcurrency, maxQueue := app.QueuePolicy()
	queue := newQueueManager(maxConcurrency, maxQueue)
	requestRegistry := newRequestRegistry()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/schema", handleSchema(app))
	mux.HandleFunc("GET /api/assets/{id}", handleAsset(store))
	mux.HandleFunc("GET /api/voice/{id}/ws", handleVoice(app, store))
	mux.HandleFunc("POST /api/predict", handlePredict(app, store, queue, requestRegistry))
	mux.HandleFunc("POST /api/stream", handleStream(app, store, queue, requestRegistry))
	mux.HandleFunc("POST /api/event", handleEvent(app, store, queue, requestRegistry))
	mux.HandleFunc("POST /api/cancel", handleCancel(requestRegistry))
	logger := app.Logger()
	mux.HandleFunc("POST /api/upload", handleUpload(logger, store))
	mux.Handle("/", handleFrontend())

	return &appHandler{
		next:     logRequests(logger, mux),
		store:    store,
		requests: requestRegistry,
	}
}

var voiceUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func handleSchema(app *core.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, app.Schema())
	}
}

func handlePredict(
	app *core.App,
	store *assetStore,
	queue *queueManager,
	registry *requestRegistry,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := app.Logger()
		rawContext := r.Context()
		handlerContext, handlerCancel := context.WithCancel(rawContext)
		requestID := requestIDFromContext(rawContext)
		if requestID == "" {
			requestID = newRequestID()
		}
		if registry != nil {
			registry.register(requestID, handlerCancel)
			defer registry.unregister(requestID)
		}

		request, err := decodePredictRequest(r)
		if err != nil {
			warnRequest(logger, r, "predict request decode failed", "error", err)
			writeError(w, http.StatusBadRequest, err)
			return
		}

		iface, ok := app.GetInterface(request.InterfaceID)
		if !ok {
			err := fmt.Errorf("interface %q not found", request.InterfaceID)
			warnRequest(logger, r, "predict interface not found", "interface_id", request.InterfaceID, "error", err)
			writeError(w, http.StatusNotFound, err)
			return
		}

		if iface.Handler == nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("interface %q does not have handler", iface.ID))
			return
		}

		release, _, err := queue.acquire(handlerContext, iface.ID)
		if errors.Is(err, errQueueFull) {
			writeError(w, http.StatusTooManyRequests, errQueueFull)
			return
		}
		if err != nil {
			if errors.Is(err, errQueueFull) {
				writeError(w, http.StatusTooManyRequests, errQueueFull)
				return
			}

			if errors.Is(err, context.Canceled) {
				writeError(w, http.StatusRequestTimeout, err)
			} else {
				writeError(w, http.StatusRequestTimeout, err)
			}
			return
		}
		defer release()

		flattenedInputs := flattenLeafComponents(iface.Inputs)
		requestData := mergeStateInputs(handlerContext, app, iface.ID, flattenedInputs, request.Data)

		hydrated, err := hydrateAssets(flattenedInputs, requestData, store)
		if err != nil {
			warnRequest(logger, r, "predict asset hydration failed", "interface_id", request.InterfaceID, "error", err)
			writeError(w, http.StatusBadRequest, err)
			return
		}

		data, err := iface.Handler.Invoke(handlerContext, hydrated)
		if err != nil {
			status := http.StatusBadRequest
			if isPanicError(err) {
				status = http.StatusInternalServerError
				errorRequest(logger, r, "predict handler panicked", "interface_id", request.InterfaceID, "error", err)
			} else {
				warnRequest(logger, r, "predict handler failed", "interface_id", request.InterfaceID, "error", err)
			}
			writeError(w, status, err)
			return
		}

		responseData, err := dehydrateOutputs(iface.Outputs, data, store)
		if err != nil {
			errorRequest(logger, r, "predict output dehydration failed", "interface_id", request.InterfaceID, "error", err)
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		updateStateFromOutputs(app, iface.ID, iface.Outputs, data)

		writeJSON(w, http.StatusOK, predictResponse{Data: responseData})
	}
}

func handleEvent(
	app *core.App,
	store *assetStore,
	queue *queueManager,
	registry *requestRegistry,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := app.Logger()
		rawContext := r.Context()
		handlerContext, handlerCancel := context.WithCancel(rawContext)
		requestID := requestIDFromContext(rawContext)
		if requestID == "" {
			requestID = newRequestID()
		}
		if registry != nil {
			registry.register(requestID, handlerCancel)
			defer registry.unregister(requestID)
		}

		request, err := decodeEventRequest(r)
		if err != nil {
			warnRequest(logger, r, "event request decode failed", "error", err)
			writeError(w, http.StatusBadRequest, err)
			return
		}

		iface, event, ok := app.GetEvent(request.InterfaceID, request.EventID)
		if !ok || iface.Kind != "blocks" {
			err := fmt.Errorf("event %q not found", request.EventID)
			warnRequest(
				logger,
				r,
				"event binding not found",
				"interface_id",
				request.InterfaceID,
				"event_id",
				request.EventID,
				"error",
				err,
			)
			writeError(w, http.StatusNotFound, err)
			return
		}
		if event.Handler == nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("event %q does not have handler", event.ID))
			return
		}

		release, _, err := queue.acquire(handlerContext, iface.ID)
		if errors.Is(err, errQueueFull) {
			writeError(w, http.StatusTooManyRequests, errQueueFull)
			return
		}
		if err != nil {
			writeError(w, http.StatusRequestTimeout, err)
			return
		}
		defer release()

		inputComponents := componentsByIDs(iface.Components, event.Inputs)
		outputComponents := componentsByIDs(iface.Components, event.Outputs)
		requestData := valuesForComponents(inputComponents, request.Data)
		requestData = mergeStateInputs(handlerContext, app, iface.ID, inputComponents, requestData)

		hydrated, err := hydrateAssets(inputComponents, requestData, store)
		if err != nil {
			warnRequest(
				logger,
				r,
				"event asset hydration failed",
				"interface_id",
				request.InterfaceID,
				"event_id",
				request.EventID,
				"error",
				err,
			)
			writeError(w, http.StatusBadRequest, err)
			return
		}

		values, err := event.Handler.Invoke(handlerContext, hydrated)
		if err != nil {
			status := http.StatusBadRequest
			if isPanicError(err) {
				status = http.StatusInternalServerError
				errorRequest(
					logger,
					r,
					"event handler panicked",
					"interface_id",
					request.InterfaceID,
					"event_id",
					request.EventID,
					"error",
					err,
				)
			} else {
				warnRequest(
					logger,
					r,
					"event handler failed",
					"interface_id",
					request.InterfaceID,
					"event_id",
					request.EventID,
					"error",
					err,
				)
			}
			writeError(w, status, err)
			return
		}

		responseData, err := dehydrateEventOutputs(outputComponents, values, store)
		if err != nil {
			errorRequest(
				logger,
				r,
				"event output dehydration failed",
				"interface_id",
				request.InterfaceID,
				"event_id",
				request.EventID,
				"error",
				err,
			)
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		updateStateFromOutputs(app, iface.ID, outputComponents, values)

		writeJSON(w, http.StatusOK, eventResponse{Data: mapOutputsByID(outputComponents, responseData)})
	}
}

func handleStream(
	app *core.App,
	store *assetStore,
	queue *queueManager,
	registry *requestRegistry,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := app.Logger()
		rawContext := r.Context()
		handlerContext, handlerCancel := context.WithCancel(rawContext)
		requestID := requestIDFromContext(rawContext)
		if requestID == "" {
			requestID = newRequestID()
		}
		if registry != nil {
			registry.register(requestID, handlerCancel)
			defer registry.unregister(requestID)
		}

		request, err := decodePredictRequest(r)
		if err != nil {
			warnRequest(logger, r, "stream request decode failed", "error", err)
			writeError(w, http.StatusBadRequest, err)
			return
		}

		iface, ok := app.GetInterface(request.InterfaceID)
		if !ok {
			err := fmt.Errorf("interface %q not found", request.InterfaceID)
			warnRequest(logger, r, "stream interface not found", "interface_id", request.InterfaceID, "error", err)
			writeError(w, http.StatusNotFound, err)
			return
		}

		if iface.Handler == nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("interface %q does not have stream handler", iface.ID))
			return
		}

		release, queued, err := queue.acquire(handlerContext, iface.ID)
		if errors.Is(err, errQueueFull) {
			writeError(w, http.StatusTooManyRequests, errQueueFull)
			return
		}
		if err != nil {
			writeError(w, http.StatusRequestTimeout, err)
			return
		}
		defer release()

		flusher, _ := w.(http.Flusher)
		requestLog := requestID
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set(requestIDHeader, requestLog)

		if queued {
			sendStreamStatus(w, "queued", requestLog, map[string]any{"status": "queued", "request_id": requestLog, "queued": true}, flusher)
		}
		sendStreamStatus(w, "running", requestLog, map[string]any{
			"status":     "running",
			"request_id": requestLog,
			"queued":     queued,
		}, flusher)

		flattenedInputs := flattenLeafComponents(iface.Inputs)
		requestData := mergeStateInputs(handlerContext, app, iface.ID, flattenedInputs, request.Data)

		hydrated, err := hydrateAssets(flattenedInputs, requestData, store)
		if err != nil {
			warnRequest(logger, r, "stream asset hydration failed", "interface_id", request.InterfaceID, "error", err)
			writeError(w, http.StatusBadRequest, err)
			return
		}

		if flusher != nil {
			flusher.Flush()
		}

		err = iface.Handler.Stream(handlerContext, hydrated, func(value any) {
			sendStreamData(w, "running", value)
			if flusher != nil {
				flusher.Flush()
			}
		})
		if err != nil {
			if isPanicError(err) {
				errorRequest(logger, r, "stream handler panicked", "interface_id", request.InterfaceID, "error", err)
				sendStreamStatus(w, "error", requestLog, map[string]any{"status": "error", "error": err.Error()}, flusher)
			} else {
				warnRequest(logger, r, "stream handler failed", "interface_id", request.InterfaceID, "error", err)
				sendStreamStatus(w, "error", requestLog, map[string]any{"status": "error", "error": err.Error()}, flusher)
			}
		} else {
			sendStreamStatus(w, "done", requestLog, map[string]any{"status": "done", "request_id": requestLog}, flusher)
		}
	}
}

func handleUpload(logger *slog.Logger, store *assetStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			warnRequest(logger, r, "upload multipart parse failed", "error", err)
			writeError(w, http.StatusBadRequest, err)
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			warnRequest(logger, r, "upload file missing", "error", err)
			writeError(w, http.StatusBadRequest, err)
			return
		}
		defer file.Close()

		record, err := store.create(header.Filename, header.Header.Get("Content-Type"), file)
		if err != nil {
			warnRequest(logger, r, "upload file read failed", "file_name", header.Filename, "error", err)
			writeError(w, http.StatusBadRequest, err)
			return
		}

		asset := record.browserValue()
		writeJSON(w, http.StatusOK, uploadResponse(asset))
	}
}

func handleCancel(registry *requestRegistry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request cancelRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid cancel request"))
			return
		}

		if request.RequestID == "" {
			writeError(w, http.StatusBadRequest, errors.New("request_id is required"))
			return
		}

		if registry == nil || !registry.cancel(request.RequestID) {
			writeError(w, http.StatusNotFound, fmt.Errorf("request %q not found", request.RequestID))
			return
		}

		writeJSON(w, http.StatusOK, cancelResponse{
			RequestID: request.RequestID,
			Cancelled: true,
		})
	}
}

func handleAsset(store *assetStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		record, ok := store.get(r.PathValue("id"))
		if !ok {
			writeError(w, http.StatusNotFound, fmt.Errorf("asset %q not found", r.PathValue("id")))
			return
		}

		file, err := os.Open(record.Path)
		if err != nil {
			writeError(w, http.StatusNotFound, fmt.Errorf("asset %q not found", record.ID))
			return
		}
		defer file.Close()

		w.Header().Set("Content-Type", record.ContentType)
		http.ServeContent(w, r, record.Name, time.Time{}, file)
	}
}

func handleVoice(app *core.App, store *assetStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := app.Logger()
		iface, ok := app.GetInterface(r.PathValue("id"))
		if !ok || iface.Kind != "voice" || iface.VoiceHandler == nil {
			writeError(w, http.StatusNotFound, fmt.Errorf("interface %q not found", r.PathValue("id")))
			return
		}

		conn, err := voiceUpgrader.Upgrade(w, r, nil)
		if err != nil {
			warnRequest(logger, r, "voice websocket upgrade failed", "interface_id", r.PathValue("id"), "error", err)
			return
		}
		defer conn.Close()

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		incoming := make(chan runtime.VoiceEvent, 16)
		outgoing := make(chan runtime.VoiceOutbound, 16)
		readErrCh := make(chan error, 1)
		handlerErrCh := make(chan error, 1)

		go readVoiceMessages(ctx, conn, incoming, readErrCh)
		go func() {
			handlerErrCh <- iface.VoiceHandler.Run(ctx, runtime.NewVoiceSession(ctx, incoming, outgoing))
			close(outgoing)
		}()

		for outbound := range outgoing {
			message, err := buildVoiceMessage(outbound, store)
			if err != nil {
				errorRequest(logger, r, "voice output build failed", "interface_id", iface.ID, "error", err)
				_ = conn.WriteJSON(map[string]any{"type": "error", "text": err.Error()})
				cancel()
				break
			}
			if err := conn.WriteJSON(message); err != nil {
				cancel()
				break
			}
		}

		cancel()
		_ = conn.Close()
		if err := <-handlerErrCh; err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, websocket.ErrCloseSent) {
			warnRequest(logger, r, "voice handler failed", "interface_id", iface.ID, "error", err)
		}
		if err := <-readErrCh; err != nil && !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
			warnRequest(logger, r, "voice websocket read failed", "interface_id", iface.ID, "error", err)
		}
	}
}

func decodePredictRequest(r *http.Request) (predictRequest, error) {
	defer r.Body.Close()

	var request predictRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		return predictRequest{}, err
	}
	if request.InterfaceID == "" {
		return predictRequest{}, errors.New("interface_id is required")
	}
	if request.Data == nil {
		request.Data = []any{}
	}

	return request, nil
}

func decodeEventRequest(r *http.Request) (eventRequest, error) {
	defer r.Body.Close()

	var request eventRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		return eventRequest{}, err
	}
	if request.InterfaceID == "" {
		return eventRequest{}, errors.New("interface_id is required")
	}
	if request.EventID == "" {
		return eventRequest{}, errors.New("event_id is required")
	}
	if request.Data == nil {
		request.Data = map[string]any{}
	}

	return request, nil
}

func handleFrontend() http.Handler {
	subtree, err := fs.Sub(frontendAssets, "assets")
	if err != nil {
		return http.NotFoundHandler()
	}

	fileServer := http.FileServer(http.FS(subtree))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			index, err := fs.ReadFile(subtree, "index.html")
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(index)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

func logRequests(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		r, requestID := withRequestID(w, r)
		recorder := &responseRecorder{ResponseWriter: w}

		next.ServeHTTP(recorder, r)

		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}

		logger.InfoContext(
			r.Context(),
			"http request completed",
			"request_id", requestID,
			"method", r.Method,
			"path", r.URL.Path,
			"status", status,
			"duration_ms", time.Since(started).Milliseconds(),
		)
	})
}

func withRequestID(w http.ResponseWriter, r *http.Request) (*http.Request, string) {
	requestID := strings.TrimSpace(r.Header.Get(requestIDHeader))
	if requestID == "" {
		requestID = newRequestID()
	}

	w.Header().Set(requestIDHeader, requestID)
	ctx := context.WithValue(r.Context(), requestIDContextKey{}, requestID)
	return r.WithContext(ctx), requestID
}

func requestIDFromContext(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDContextKey{}).(string)
	return requestID
}

func isPanicError(err error) bool {
	var panicErr runtime.PanicError
	return errors.As(err, &panicErr)
}

func newRequestID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "req_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}

	return "req_" + hex.EncodeToString(bytes[:])
}

func warnRequest(logger *slog.Logger, r *http.Request, msg string, args ...any) {
	logger.WarnContext(r.Context(), msg, requestLogArgs(r, args...)...)
}

func errorRequest(logger *slog.Logger, r *http.Request, msg string, args ...any) {
	logger.ErrorContext(r.Context(), msg, requestLogArgs(r, args...)...)
}

func requestLogArgs(r *http.Request, args ...any) []any {
	if requestID := requestIDFromContext(r.Context()); requestID != "" {
		return append([]any{"request_id", requestID}, args...)
	}

	return args
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, errorResponse{
		Error: responseError{
			Code:    errorCode(status),
			Message: err.Error(),
		},
	})
}

func errorCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "bad_request"
	case http.StatusRequestTimeout:
		return "request_timeout"
	case http.StatusTooManyRequests:
		return "queue_full"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusInternalServerError:
		return "internal_error"
	default:
		return "error"
	}
}

func encodeEventValue(value any) string {
	text, ok := value.(string)
	if ok {
		return strings.ReplaceAll(text, "\n", "\\n")
	}

	payload, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}

	return string(payload)
}

func sendStreamStatus(w http.ResponseWriter, event string, requestID string, payload map[string]any, flusher http.Flusher) {
	if requestID != "" {
		payload["request_id"] = requestID
	}
	payload["status"] = event

	_ = writeServerSentEvent(w, event, payload)
}

func sendStreamData(w http.ResponseWriter, status string, value any) {
	payload := map[string]any{
		"status": status,
		"data":   value,
	}
	_ = writeServerSentEvent(w, "data", payload)
}

func writeServerSentEvent(w http.ResponseWriter, event string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, strings.ReplaceAll(string(data), "\n", "\\n"))
	return err
}

func sanitizeFileName(name string) string {
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-")
	return replacer.Replace(name)
}

func flattenLeafComponents(components []component.Component) []component.Component {
	leafs := make([]component.Component, 0, len(components))
	for _, component := range components {
		if isLayoutComponent(component.Type) {
			leafs = append(leafs, flattenLeafComponents(component.Items)...)
			continue
		}
		leafs = append(leafs, component)
	}
	return leafs
}

func isLayoutComponent(kind string) bool {
	switch kind {
	case "row", "column", "group":
		return true
	default:
		return false
	}
}

func fileInputFromRecord(record media.FileInput, kind string) any {
	switch kind {
	case "audio":
		return media.AudioInput{
			ID:          record.ID,
			Name:        record.Name,
			Size:        record.Size,
			ContentType: record.ContentType,
			Path:        record.Path,
			URL:         record.URL,
		}
	case "image":
		return media.ImageInput{
			FileInput: media.FileInput{
				ID:          record.ID,
				Name:        record.Name,
				Size:        record.Size,
				ContentType: record.ContentType,
				Path:        record.Path,
				URL:         record.URL,
			},
		}
	default:
		return media.FileInput{
			ID:          record.ID,
			Name:        record.Name,
			Size:        record.Size,
			ContentType: record.ContentType,
			Path:        record.Path,
			URL:         record.URL,
		}
	}
}

func mergeStateInputs(_ context.Context, app *core.App, interfaceID string, inputs []component.Component, data []any) []any {
	merged := make([]any, len(inputs))
	for index, component := range inputs {
		var value any
		if index < len(data) {
			value = data[index]
		}

		if component.Type == "state" {
			if value == nil {
				if stateValue, ok := app.State(interfaceID, component.ID); ok {
					value = stateValue
				} else if value = component.Props["default"]; value == nil {
					value = nil
				}
			}
			app.SetState(interfaceID, component.ID, value)
		}

		merged[index] = value
	}

	if len(data) > len(inputs) {
		merged = append(merged, data[len(inputs):]...)
	}

	return merged
}

func updateStateFromOutputs(app *core.App, interfaceID string, outputs []component.Component, values []any) {
	flatOutputs := flattenLeafComponents(outputs)
	for index, component := range flatOutputs {
		if component.Type != "state" {
			continue
		}
		if index >= len(values) {
			app.SetState(interfaceID, component.ID, nil)
			continue
		}
		app.SetState(interfaceID, component.ID, values[index])
	}
}

func componentsByIDs(components []component.Component, ids []string) []component.Component {
	flatComponents := flattenLeafComponents(components)
	byID := make(map[string]component.Component, len(flatComponents))
	for _, item := range flatComponents {
		byID[item.ID] = item
	}

	result := make([]component.Component, 0, len(ids))
	for _, id := range ids {
		if item, ok := byID[id]; ok {
			result = append(result, item)
		}
	}
	return result
}

func valuesForComponents(components []component.Component, values map[string]any) []any {
	result := make([]any, 0, len(components))
	for _, item := range components {
		result = append(result, values[item.ID])
	}
	return result
}

func mapOutputsByID(components []component.Component, values []any) map[string]any {
	result := make(map[string]any, len(components))
	for index, item := range components {
		if index >= len(values) {
			result[item.ID] = nil
			continue
		}
		result[item.ID] = values[index]
	}
	return result
}

func hydrateAssets(inputs []component.Component, data []any, store *assetStore) ([]any, error) {
	hydrated := make([]any, 0, len(data))

	for index, item := range data {
		if index >= len(inputs) {
			hydrated = append(hydrated, item)
			continue
		}

		component := inputs[index]
		if component.Type != "audio" && component.Type != "file" && component.Type != "image" {
			hydrated = append(hydrated, item)
			continue
		}

		descriptor, ok := item.(map[string]any)
		if !ok {
			hydrated = append(hydrated, item)
			continue
		}

		id, _ := descriptor["id"].(string)
		if id == "" {
			hydrated = append(hydrated, item)
			continue
		}

		record, found := store.get(id)
		if !found {
			return nil, fmt.Errorf("asset %q not found", id)
		}
		hydrated = append(hydrated, fileInputFromRecord(record.handlerValue(), component.Type))
	}

	return hydrated, nil
}

func dehydrateOutputs(components []component.Component, values []any, store *assetStore) ([]any, error) {
	flatComponents := flattenLeafComponents(components)

	result := make([]any, 0, len(values))
	for index, value := range values {
		if index >= len(flatComponents) {
			result = append(result, value)
			continue
		}

		component := flatComponents[index]
		if component.Type != "audio" && component.Type != "file" && component.Type != "image" {
			result = append(result, value)
			continue
		}

		output, ok, err := asFileOutput(value)
		if err != nil {
			return nil, err
		}
		if !ok {
			result = append(result, value)
			continue
		}

		record, err := store.createFromPath(output)
		if err != nil {
			return nil, err
		}
		result = append(result, record.browserValue())
	}

	return result, nil
}

func dehydrateEventOutputs(components []component.Component, values []any, store *assetStore) ([]any, error) {
	result := make([]any, 0, len(values))
	for index, value := range values {
		if update, ok := value.(runtime.Update); ok {
			envelope, err := updateEnvelope(update, componentAt(components, index), store)
			if err != nil {
				return nil, err
			}
			result = append(result, envelope)
			continue
		}

		dehydrated, err := dehydrateOutputs(componentAt(components, index), []any{value}, store)
		if err != nil {
			return nil, err
		}
		if len(dehydrated) == 0 {
			result = append(result, value)
			continue
		}
		result = append(result, dehydrated[0])
	}
	return result, nil
}

func componentAt(components []component.Component, index int) []component.Component {
	if index < 0 || index >= len(components) {
		return nil
	}
	return []component.Component{components[index]}
}

func updateEnvelope(update runtime.Update, components []component.Component, store *assetStore) (map[string]any, error) {
	payload := map[string]any{"kind": runtime.UpdateKind}
	if update.Value != nil {
		value := update.Value
		if len(components) > 0 {
			dehydrated, err := dehydrateOutputs(components, []any{update.Value}, store)
			if err != nil {
				return nil, err
			}
			if len(dehydrated) > 0 {
				value = dehydrated[0]
			}
		}
		payload["value"] = value
	}
	if update.Visible != nil {
		payload["visible"] = *update.Visible
	}
	if update.Disabled != nil {
		payload["disabled"] = *update.Disabled
	}
	if update.Choices != nil {
		payload["choices"] = append([]string{}, update.Choices...)
	}
	if update.Label != nil {
		payload["label"] = *update.Label
	}
	return payload, nil
}

func asFileOutput(value any) (media.FileLikeOutput, bool, error) {
	switch output := value.(type) {
	case media.AudioOutput:
		return media.FileLikeOutput(output), output.Path != "", nil
	case *media.AudioOutput:
		if output == nil {
			return media.FileLikeOutput{}, false, nil
		}
		return media.FileLikeOutput(*output), output.Path != "", nil
	default:
		payload, err := json.Marshal(value)
		if err != nil {
			return media.FileLikeOutput{}, false, err
		}
		var decoded media.FileOutput
		if err := json.Unmarshal(payload, &decoded); err != nil {
			return media.FileLikeOutput{}, false, err
		}
		return media.FileLikeOutput(decoded), decoded.Path != "", nil
	}
}

func readVoiceMessages(ctx context.Context, conn *websocket.Conn, incoming chan<- runtime.VoiceEvent, errCh chan<- error) {
	defer close(incoming)
	defer close(errCh)

	for {
		var event runtime.VoiceEvent
		if err := conn.ReadJSON(&event); err != nil {
			errCh <- err
			return
		}

		select {
		case <-ctx.Done():
			errCh <- ctx.Err()
			return
		case incoming <- event:
		}
	}
}

func buildVoiceMessage(outbound runtime.VoiceOutbound, store *assetStore) (map[string]any, error) {
	if outbound.AudioOutput != nil {
		record, err := store.createFromPath(*outbound.AudioOutput)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"type":  "output.audio",
			"audio": record.browserValue(),
		}, nil
	}

	if outbound.Event == nil {
		return nil, errors.New("voice outbound event is empty")
	}

	payload, err := json.Marshal(outbound.Event)
	if err != nil {
		return nil, err
	}

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}
