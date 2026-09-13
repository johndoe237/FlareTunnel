package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStreamHTTPResponseFramesUnknownLengthBody(t *testing.T) {
	resp := &http.Response{
		StatusCode:    http.StatusOK,
		Status:        "200 OK",
		Header:        make(http.Header),
		ContentLength: -1,
		Body:          io.NopCloser(strings.NewReader("data: token\n\n")),
	}
	resp.Header.Set("Content-Type", "text/event-stream")
	var got bytes.Buffer
	if err := streamHTTPResponse(&got, resp); err != nil {
		t.Fatalf("streamHTTPResponse() error = %v", err)
	}
	want := "HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nTransfer-Encoding: chunked\r\n\r\n" + "d\r\ndata: token\n\n\r\n0\r\n\r\n"
	if got.String() != want {
		t.Fatalf("response = %q, want %q", got.String(), want)
	}
}

type chunkedReader struct {
	chunks chan string
}

func (r *chunkedReader) Read(p []byte) (int, error) {
	chunk, ok := <-r.chunks
	if !ok {
		return 0, io.EOF
	}
	return copy(p, chunk), nil
}

func TestStreamResponseFlushesSSEChunksImmediately(t *testing.T) {
	body := &chunkedReader{chunks: make(chan string)}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		streamResponse(w, body)
	}))
	defer server.Close()

	client := &http.Client{}
	respCh := make(chan *http.Response, 1)
	errCh := make(chan error, 1)
	go func() {
		resp, err := client.Get(server.URL)
		if err != nil {
			errCh <- err
			return
		}
		respCh <- resp
	}()
	firstChunk := "data: first\n\n"
	go func() { body.chunks <- firstChunk }()

	var resp *http.Response
	select {
	case err := <-errCh:
		t.Fatal(err)
	case resp = <-respCh:
	case <-time.After(time.Second):
		t.Fatal("request headers were not returned")
	}
	defer resp.Body.Close()

	first := make([]byte, len(firstChunk))
	if _, err := io.ReadFull(resp.Body, first); err != nil {
		t.Fatalf("reading first SSE chunk: %v", err)
	}
	if !strings.Contains(string(first), "data: first") {
		t.Fatalf("first chunk = %q", first)
	}

	body.chunks <- "data: second\n\n"
	close(body.chunks)
	remaining, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading remaining SSE stream: %v", err)
	}
	if !strings.Contains(string(remaining), "data: second") {
		t.Fatalf("remaining stream = %q", remaining)
	}
}

func TestWorkerScriptRelaysResponseBodyWithoutParsing(t *testing.T) {
	if !strings.Contains(WorkerScript, "return new Response(response.body") {
		t.Fatal("WorkerScript does not return the upstream response body directly")
	}
	if strings.Contains(WorkerScript, "JSON.parse") || strings.Contains(WorkerScript, "response.body.getReader") {
		t.Fatal("WorkerScript parses or manually consumes the upstream response body")
	}
	if !strings.Contains(WorkerScript, "hopByHopHeaders") || strings.Contains(WorkerScript, "const allowedHeaders") {
		t.Fatal("WorkerScript does not transparently preserve provider-specific request headers")
	}
}
