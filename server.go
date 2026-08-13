// Package htxtd serves static HyperTexting website output.
package htxtd

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Server serves a static website directory over HTTP.
type Server struct {
	Directory  string
	Bind       string
	Port       int
	LiveReload bool
	LogWriter  io.Writer

	reload reloadHub
	log    sync.Mutex
}

type reloadHub struct {
	mu      sync.Mutex
	clients map[*reloadClient]struct{}
}

type reloadClient struct {
	reloads chan struct{}
	done    chan struct{}
}

const (
	defaultReadHeaderTimeout = 5 * time.Second
	defaultShutdownTimeout   = 5 * time.Second
)

// Address returns the TCP address on which the server listens.
func (server *Server) Address() string {
	return net.JoinHostPort(server.Bind, strconv.Itoa(server.Port))
}

// Handler returns the configured static-file handler.
func (server *Server) Handler() http.Handler {
	files := http.FileServer(http.Dir(server.Directory))
	var handler http.Handler = files
	if !server.LiveReload {
		return server.logRequests(handler)
	}
	handler = http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/.htxtd/events" {
			server.serveEvents(writer, request)
			return
		}
		writer.Header().Set("Cache-Control", "no-store")
		server.serveFiles(files, writer, request)
	})
	return server.logRequests(handler)
}

// Reload notifies connected live-reload clients after a successful build.
func (server *Server) Reload() {
	server.reload.mu.Lock()
	defer server.reload.mu.Unlock()
	for client := range server.reload.clients {
		select {
		case client.reloads <- struct{}{}:
		default:
		}
	}
}

// Listen validates the server and opens its TCP listener.
func (server *Server) Listen() (net.Listener, error) {
	if err := server.Validate(); err != nil {
		return nil, err
	}
	return net.Listen("tcp", server.Address())
}

// Run validates the server and serves until the context is canceled.
func (server *Server) Run(ctx context.Context) error {
	listener, err := server.Listen()
	if err != nil {
		return err
	}
	return server.Serve(ctx, listener)
}

// Serve serves on a listener returned by Listen until the context is canceled.
func (server *Server) Serve(ctx context.Context, listener net.Listener) error {
	if listener == nil {
		return fmt.Errorf("listener is required")
	}
	httpServer := &http.Server{
		Handler:           server.Handler(),
		ReadHeaderTimeout: defaultReadHeaderTimeout,
	}
	errors := make(chan error, 1)
	go func() {
		errors <- httpServer.Serve(listener)
	}()
	select {
	case err := <-errors:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	case <-ctx.Done():
		server.closeReloadClients()
		shutdownContext, cancel := context.WithTimeout(context.Background(), defaultShutdownTimeout)
		defer cancel()
		if err := httpServer.Shutdown(shutdownContext); err != nil {
			_ = httpServer.Close()
		}
		return nil
	}
}

// Validate verifies that the server can serve a static website directory.
func (server *Server) Validate() error {
	if server.Directory == "" {
		return fmt.Errorf("website directory is required")
	}
	info, err := os.Stat(server.Directory)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("website path %q is not a directory", server.Directory)
	}
	if server.Bind == "" {
		return fmt.Errorf("bind address is required")
	}
	if server.Port < 1 || server.Port > 65535 {
		return fmt.Errorf("port %d is outside the valid range", server.Port)
	}
	return nil
}

func (server *Server) serveEvents(writer http.ResponseWriter, request *http.Request) {
	flusher, ok := writer.(http.Flusher)
	if !ok {
		http.Error(writer, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	client := &reloadClient{
		reloads: make(chan struct{}, 1),
		done:    make(chan struct{}),
	}
	server.reload.mu.Lock()
	if server.reload.clients == nil {
		server.reload.clients = map[*reloadClient]struct{}{}
	}
	server.reload.clients[client] = struct{}{}
	server.reload.mu.Unlock()
	defer func() {
		server.reload.mu.Lock()
		delete(server.reload.clients, client)
		server.reload.mu.Unlock()
	}()
	writer.Header().Set("Cache-Control", "no-cache")
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set("Connection", "keep-alive")
	flusher.Flush()
	for {
		select {
		case <-client.reloads:
			_, _ = fmt.Fprint(writer, "event: reload\ndata: {}\n\n")
			flusher.Flush()
		case <-client.done:
			return
		case <-request.Context().Done():
			return
		}
	}
}

func (server *Server) closeReloadClients() {
	server.reload.mu.Lock()
	defer server.reload.mu.Unlock()
	for client := range server.reload.clients {
		close(client.done)
		delete(server.reload.clients, client)
	}
}

func (server *Server) logRequests(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		recorder := &loggingResponseWriter{ResponseWriter: writer}
		handler.ServeHTTP(recorder, request)
		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		server.logRequest(request, status)
	})
}

func (server *Server) logRequest(request *http.Request, status int) {
	writer := server.LogWriter
	if writer == nil {
		writer = os.Stdout
	}
	server.log.Lock()
	defer server.log.Unlock()
	_, _ = fmt.Fprintf(writer, "%s  %d  %s\n", request.Method, status, request.URL.RequestURI())
}

func (server *Server) serveFiles(files http.Handler, writer http.ResponseWriter, request *http.Request) {
	recorder := &responseRecorder{
		header: http.Header{},
	}
	files.ServeHTTP(recorder, request)
	for key, values := range recorder.header {
		writer.Header()[key] = values
	}
	status := recorder.status
	if status == 0 {
		status = http.StatusOK
	}
	if request.Method != http.MethodGet || status != http.StatusOK || recorder.header.Get("Content-Range") != "" || !strings.HasPrefix(recorder.header.Get("Content-Type"), "text/html") {
		writer.WriteHeader(status)
		_, _ = writer.Write(recorder.body)
		return
	}
	writer.Header().Del("Content-Length")
	writer.WriteHeader(status)
	_, _ = writer.Write(injectReloadClient(recorder.body))
}

type responseRecorder struct {
	header http.Header
	body   []byte
	status int
}

type loggingResponseWriter struct {
	http.ResponseWriter

	status int
}

func (writer *loggingResponseWriter) Write(body []byte) (int, error) {
	if writer.status == 0 {
		writer.status = http.StatusOK
	}
	return writer.ResponseWriter.Write(body)
}

func (writer *loggingResponseWriter) WriteHeader(status int) {
	if writer.status == 0 {
		writer.status = status
	}
	writer.ResponseWriter.WriteHeader(status)
}

func (writer *loggingResponseWriter) Flush() {
	if writer.status == 0 {
		writer.status = http.StatusOK
	}
	if flusher, ok := writer.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (recorder *responseRecorder) Header() http.Header {
	return recorder.header
}

func (recorder *responseRecorder) Write(body []byte) (int, error) {
	if recorder.status == 0 {
		recorder.status = http.StatusOK
	}
	recorder.body = append(recorder.body, body...)
	return len(body), nil
}

func (recorder *responseRecorder) WriteHeader(status int) {
	if recorder.status == 0 {
		recorder.status = status
	}
}

func injectReloadClient(body []byte) []byte {
	script := []byte("<script>new EventSource('/.htxtd/events').addEventListener('reload',()=>location.reload())</script>")
	if index := strings.LastIndex(strings.ToLower(string(body)), "</body>"); index >= 0 {
		return append(append(body[:index:index], script...), body[index:]...)
	}
	return append(body, script...)
}

// AbsoluteDirectory resolves a directory from a working directory.
func AbsoluteDirectory(workingDirectory, directory string) (string, error) {
	if directory == "" {
		return "", fmt.Errorf("website directory is required")
	}
	if !filepath.IsAbs(directory) {
		directory = filepath.Join(workingDirectory, directory)
	}
	return filepath.Abs(directory)
}
