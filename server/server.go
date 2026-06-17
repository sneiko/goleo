package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sneiko/goleo/core"
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

// New creates the complete HTTP handler for an app.
func New(app *core.App) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/schema", handleSchema(app))
	mux.HandleFunc("POST /api/predict", handlePredict(app))
	mux.HandleFunc("POST /api/stream", handleStream(app))
	logger := app.Logger()
	mux.HandleFunc("POST /api/upload", handleUpload(logger))
	mux.Handle("/", handleFrontend())

	return logRequests(logger, mux)
}

func handleSchema(app *core.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, app.Schema())
	}
}

func handlePredict(app *core.App) http.HandlerFunc {
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

		data, err := iface.Handler.Invoke(r.Context(), request.Data)
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

		writeJSON(w, http.StatusOK, predictResponse{Data: data})
	}
}

func handleStream(app *core.App) http.HandlerFunc {
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

		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, _ := w.(http.Flusher)
		err = iface.Handler.Stream(r.Context(), request.Data, func(value any) {
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

func handleUpload(logger *slog.Logger) http.HandlerFunc {
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

		size, err := io.Copy(io.Discard, file)
		if err != nil {
			warnRequest(logger, r, "upload file read failed", "file_name", header.Filename, "error", err)
			writeError(w, http.StatusBadRequest, err)
			return
		}

		writeJSON(w, http.StatusOK, uploadResponse{
			ID:          "upload-" + sanitizeFileName(header.Filename) + "-" + strconv.FormatInt(size, 10),
			Name:        header.Filename,
			Size:        size,
			ContentType: header.Header.Get("Content-Type"),
		})
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
