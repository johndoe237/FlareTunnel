package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func createTestCA(t *testing.T, dir string) (string, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPath := filepath.Join(dir, "ca.crt")
	keyPath := filepath.Join(dir, "ca.key")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0o600); err != nil {
		t.Fatal(err)
	}
	return certPath, keyPath
}

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

func TestCONNECTStreamsTwentySSEEventsProgressively(t *testing.T) {
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("worker response does not support flushing")
			return
		}
		for i := 0; i < 20; i++ {
			_, _ = fmt.Fprintf(w, "data: %d\n\n", i)
			flusher.Flush()
			time.Sleep(200 * time.Millisecond)
		}
	}))
	defer worker.Close()

	dir := t.TempDir()
	certPath, keyPath := createTestCA(t, dir)
	proxy := NewProxyServer("127.0.0.1", 0)
	proxy.CACertPath = certPath
	proxy.CAKeyPath = keyPath
	proxy.ProxyAuthBasic = base64.StdEncoding.EncodeToString([]byte("user:pass"))
	proxy.UpstreamVerifySSL = false
	proxy.Workers = []*Worker{{URL: worker.URL}}
	proxyServer := httptest.NewServer(proxy.Handler())
	defer proxyServer.Close()

	proxyAddress := strings.TrimPrefix(proxyServer.URL, "http://")
	conn, err := net.Dial("tcp", proxyAddress)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := fmt.Fprintf(conn, "CONNECT api.example.test:443 HTTP/1.1\r\nHost: api.example.test:443\r\nProxy-Authorization: Basic %s\r\n\r\n", proxy.ProxyAuthBasic); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(line, "200") {
		t.Fatalf("CONNECT response = %q", line)
	}
	for {
		header, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if header == "\r\n" || header == "\n" {
			break
		}
	}
	tlsConn := tls.Client(conn, &tls.Config{InsecureSkipVerify: true, ServerName: "api.example.test"})
	if err := tlsConn.Handshake(); err != nil {
		t.Fatal(err)
	}
	defer tlsConn.Close()
	if _, err := fmt.Fprint(tlsConn, "GET /v1/chat/completions HTTP/1.1\r\nHost: api.example.test\r\nAccept: text/event-stream\r\nConnection: close\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	tlsReader := bufio.NewReader(tlsConn)
	response, err := http.ReadResponse(tlsReader, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("upstream status = %s", response.Status)
	}
	bodyReader := bufio.NewReader(response.Body)
	start := time.Now()
	last := start
	for i := 0; i < 20; i++ {
		var event strings.Builder
		for {
			part, readErr := bodyReader.ReadString('\n')
			if readErr != nil {
				t.Fatalf("event %d read: %v", i, readErr)
			}
			event.WriteString(part)
			if event.Len() >= 2 && strings.HasSuffix(event.String(), "\n\n") {
				break
			}
		}
		now := time.Now()
		if i > 0 {
			interval := now.Sub(last)
			if interval < 50*time.Millisecond || interval > 800*time.Millisecond {
				t.Fatalf("event %d interval = %s, want approximately 200ms", i, interval)
			}
		}
		last = now
		if !strings.Contains(event.String(), fmt.Sprintf("data: %d", i)) {
			t.Fatalf("event %d payload = %q", i, event.String())
		}
	}
	if elapsed := time.Since(start); elapsed < 3*time.Second {
		t.Fatalf("all events arrived in %s; expected progressive delivery over approximately 4s", elapsed)
	}
}
