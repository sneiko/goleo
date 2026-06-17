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

type responseRecorder struct {
	http.ResponseWriter
	status int
}

type appHandler struct {
	next  http.Handler
	store *assetStore
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

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/schema", handleSchema(app))
	mux.HandleFunc("GET /api/assets/{id}", handleAsset(store))
	mux.HandleFunc("GET /api/voice/{id}/ws", handleVoice(app, store))
	mux.HandleFunc("POST /api/predict", handlePredict(app, store))
	mux.HandleFunc("POST /api/stream", handleStream(app, store))
	logger := app.Logger()
	mux.HandleFunc("POST /api/upload", handleUpload(logger, store))
	mux.Handle("/", handleFrontend())

	return &appHandler{
		next:  logRequests(logger, mux),
		store: store,
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

func handlePredict(app *core.App, store *assetStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := app.Logger()
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

		hydrated, err := hydrateAssets(request.Data, store)
		if err != nil {
			warnRequest(logger, r, "predict asset hydration failed", "interface_id", request.InterfaceID, "error", err)
			writeError(w, http.StatusBadRequest, err)
			return
		}

		data, err := iface.Handler.Invoke(r.Context(), hydrated)
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

		writeJSON(w, http.StatusOK, predictResponse{Data: responseData})
	}
}

func handleStream(app *core.App, store *assetStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := app.Logger()
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

		hydrated, err := hydrateAssets(request.Data, store)
		if err != nil {
			warnRequest(logger, r, "stream asset hydration failed", "interface_id", request.InterfaceID, "error", err)
			writeError(w, http.StatusBadRequest, err)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, _ := w.(http.Flusher)
		err = iface.Handler.Stream(r.Context(), hydrated, func(value any) {
			fmt.Fprintf(w, "data: %s\n\n", encodeEventValue(value))
			if flusher != nil {
				flusher.Flush()
			}
		})
		if err != nil {
			if isPanicError(err) {
				errorRequest(logger, r, "stream handler panicked", "interface_id", request.InterfaceID, "error", err)
			} else {
				warnRequest(logger, r, "stream handler failed", "interface_id", request.InterfaceID, "error", err)
			}
			fmt.Fprintf(w, "event: error\ndata: %s\n\n", encodeEventValue(err.Error()))
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

func sanitizeFileName(name string) string {
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-")
	return replacer.Replace(name)
}

func hydrateAssets(data []any, store *assetStore) ([]any, error) {
	hydrated := make([]any, 0, len(data))
	for _, item := range data {
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
		hydrated = append(hydrated, record.handlerValue())
	}

	return hydrated, nil
}

func dehydrateOutputs(components []component.Component, values []any, store *assetStore) ([]any, error) {
	result := make([]any, 0, len(values))
	for index, value := range values {
		if index >= len(components) {
			result = append(result, value)
			continue
		}

		component := components[index]
		if component.Type != "audio" {
			result = append(result, value)
			continue
		}

		output, ok, err := asAudioOutput(value)
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

func asAudioOutput(value any) (media.AudioOutput, bool, error) {
	switch output := value.(type) {
	case media.AudioOutput:
		return output, output.Path != "", nil
	case *media.AudioOutput:
		if output == nil {
			return media.AudioOutput{}, false, nil
		}
		return *output, output.Path != "", nil
	default:
		payload, err := json.Marshal(value)
		if err != nil {
			return media.AudioOutput{}, false, err
		}
		var decoded media.AudioOutput
		if err := json.Unmarshal(payload, &decoded); err != nil {
			return media.AudioOutput{}, false, err
		}
		return decoded, decoded.Path != "", nil
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
