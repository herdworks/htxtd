package htxtd

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestServerServesStaticDirectory(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "index.html"), []byte("Hello"), 0644); err != nil {
		t.Fatal(err)
	}
	server := Server{
		Directory: directory,
		Bind:      "127.0.0.1",
		Port:      8080,
		LogWriter: io.Discard,
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if response.Body.String() != "Hello" {
		t.Fatalf("body = %q, want %q", response.Body.String(), "Hello")
	}
}

func TestServerValidateRejectsInvalidConfiguration(t *testing.T) {
	for _, server := range []*Server{
		{Bind: "localhost", Port: 8080},
		{Directory: t.TempDir(), Port: 8080},
		{Directory: t.TempDir(), Bind: "localhost", Port: 0},
	} {
		if err := server.Validate(); err == nil {
			t.Error("Validate() unexpectedly succeeded")
		}
	}
}

func TestServerLiveReloadInjectsBrowserClient(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "index.html"), []byte("<body>Hello</body>"), 0644); err != nil {
		t.Fatal(err)
	}
	server := Server{Directory: directory, Bind: "localhost", Port: 8080, LiveReload: true, LogWriter: io.Discard}
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if got := response.Body.String(); got != "<body>Hello<script>new EventSource('/.htxtd/events').addEventListener('reload',()=>location.reload())</script></body>" {
		t.Fatalf("body = %q", got)
	}
	if got, want := response.Header().Get("Cache-Control"), "no-store"; got != want {
		t.Fatalf("Cache-Control = %q, want %q", got, want)
	}
}

func TestServerLiveReloadPreservesPartialAndHeadResponses(t *testing.T) {
	directory := t.TempDir()
	body := "<body>Hello</body>"
	if err := os.WriteFile(filepath.Join(directory, "index.html"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	server := Server{Directory: directory, Bind: "localhost", Port: 8080, LiveReload: true, LogWriter: io.Discard}
	partialRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	partialRequest.Header.Set("Range", "bytes=0-4")
	partialResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(partialResponse, partialRequest)
	if got, want := partialResponse.Code, http.StatusPartialContent; got != want {
		t.Fatalf("partial status = %d, want %d", got, want)
	}
	if got, want := partialResponse.Body.String(), body[:5]; got != want {
		t.Fatalf("partial body = %q, want %q", got, want)
	}
	if got := partialResponse.Header().Get("Content-Range"); got == "" {
		t.Fatal("partial response has no Content-Range")
	}
	headResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(headResponse, httptest.NewRequest(http.MethodHead, "/", nil))
	if got, want := headResponse.Code, http.StatusOK; got != want {
		t.Fatalf("HEAD status = %d, want %d", got, want)
	}
	if got := headResponse.Body.String(); got != "" {
		t.Fatalf("HEAD body = %q, want empty body", got)
	}
	if got, want := headResponse.Header().Get("Content-Length"), strconv.Itoa(len(body)); got != want {
		t.Fatalf("HEAD Content-Length = %q, want %q", got, want)
	}
}

func TestServerLogsRequests(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "index.html"), []byte("Hello"), 0644); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	server := Server{Directory: directory, Bind: "localhost", Port: 8080, LogWriter: &logs}
	server.Handler().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if got, want := logs.String(), "GET  200  /\n"; got != want {
		t.Fatalf("log = %q, want %q", got, want)
	}
}

func TestServerAddressSupportsIPv6(t *testing.T) {
	server := Server{Bind: "::1", Port: 8080}
	if got, want := server.Address(), "[::1]:8080"; got != want {
		t.Fatalf("Address() = %q, want %q", got, want)
	}
}

func TestServerRunReturnsListenerError(t *testing.T) {
	listener := reserveListener(t)
	defer listener.Close()
	server := Server{
		Directory: t.TempDir(),
		Bind:      "127.0.0.1",
		Port:      listenerPort(t, listener),
		LogWriter: io.Discard,
	}
	if err := server.Run(context.Background()); err == nil {
		t.Fatal("Run() unexpectedly succeeded")
	}
}

func TestServerServeDeliversReloadAndShutsDownSSEClients(t *testing.T) {
	listener := reserveListener(t)
	address := listener.Addr().String()
	server := Server{
		Directory:  t.TempDir(),
		Bind:       "127.0.0.1",
		Port:       listenerPort(t, listener),
		LiveReload: true,
		LogWriter:  io.Discard,
	}
	context, cancel := context.WithCancel(context.Background())
	errors := make(chan error, 1)
	go func() {
		errors <- server.Serve(context, listener)
	}()
	response, err := http.Get("http://" + address + "/.htxtd/events")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if got, want := response.StatusCode, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	server.Reload()
	line, err := bufio.NewReader(response.Body).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if got, want := line, "event: reload\n"; got != want {
		t.Fatalf("event = %q, want %q", got, want)
	}
	cancel()
	select {
	case err := <-errors:
		if err != nil {
			t.Fatalf("Serve() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Serve() did not shut down with an open SSE client")
	}
}

func reserveListener(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return listener
}

func listenerPort(t *testing.T, listener net.Listener) int {
	t.Helper()
	_, value, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(value)
	if err != nil {
		t.Fatal(err)
	}
	return port
}
