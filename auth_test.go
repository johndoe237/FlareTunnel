package main

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidateProxyAuthBasic(t *testing.T) {
	valid := base64.StdEncoding.EncodeToString([]byte("user1:pass1"))
	if err := ValidateProxyAuthBasic(valid); err != nil {
		t.Fatalf("valid credential rejected: %v", err)
	}
	for _, value := range []string{"", "not-base64", base64.StdEncoding.EncodeToString([]byte("user1:")), base64.StdEncoding.EncodeToString([]byte(":pass1")), base64.RawStdEncoding.EncodeToString([]byte("user1:pass1"))} {
		if err := ValidateProxyAuthBasic(value); err == nil {
			t.Errorf("credential %q should be rejected", value)
		}
	}
}

func TestStartRejectsMissingOrInvalidProxyAuth(t *testing.T) {
	for _, value := range []string{"", "invalid"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv(proxyAuthBasicEnv, value)
			ps := NewProxyServer("127.0.0.1", 0)
			if err := ps.Start(""); err == nil {
				t.Fatal("Start should reject missing or invalid AUTH_PROXY_BASIC")
			}
		})
	}
}

func TestProxyAuthenticationRequiredForAllMethods(t *testing.T) {
	ps := NewProxyServer("127.0.0.1", 8080)
	ps.ProxyAuthBasic = base64.StdEncoding.EncodeToString([]byte("user1:pass1"))
	handler := ps.Handler()
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions, http.MethodConnect} {
		req := httptest.NewRequest(method, "http://proxy.example.test/", nil)
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		if resp.Code != http.StatusProxyAuthRequired {
			t.Errorf("%s without credentials returned %d, want 407", method, resp.Code)
		}
		if got := resp.Header().Get("Proxy-Authenticate"); got != "Basic" {
			t.Errorf("%s challenge = %q, want Basic", method, got)
		}
	}
}

func TestProxyAuthenticationRejectsWrongCredentialWithoutForwarding(t *testing.T) {
	ps := NewProxyServer("127.0.0.1", 8080)
	ps.ProxyAuthBasic = base64.StdEncoding.EncodeToString([]byte("user1:pass1"))
	ps.Workers = []*Worker{{URL: "http://127.0.0.1:1"}}
	req := httptest.NewRequest(http.MethodGet, "http://proxy.example.test/", nil)
	req.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("wrong:password")))
	resp := httptest.NewRecorder()
	ps.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusProxyAuthRequired {
		t.Fatalf("wrong credentials returned %d, want 407", resp.Code)
	}
}

func TestProxyAuthenticationAllowsCorrectCredentialToContinue(t *testing.T) {
	ps := NewProxyServer("127.0.0.1", 8080)
	ps.ProxyAuthBasic = base64.StdEncoding.EncodeToString([]byte("user1:pass1"))
	req := httptest.NewRequest(http.MethodGet, "http://proxy.example.test/", nil)
	req.Header.Set("Proxy-Authorization", "Basic "+ps.ProxyAuthBasic)
	resp := httptest.NewRecorder()
	ps.Handler().ServeHTTP(resp, req)
	if resp.Code == http.StatusProxyAuthRequired {
		t.Fatal("correct credentials were rejected")
	}
}

func TestConnectCorrectCredentialReachesConnectHandler(t *testing.T) {
	ps := NewProxyServer("127.0.0.1", 8080)
	ps.ProxyAuthBasic = base64.StdEncoding.EncodeToString([]byte("user1:pass1"))
	req := httptest.NewRequest(http.MethodConnect, "http://proxy.example.test:443", nil)
	req.Host = "proxy.example.test:443"
	req.Header.Set("Proxy-Authorization", "Basic "+ps.ProxyAuthBasic)
	resp := httptest.NewRecorder()
	ps.Handler().ServeHTTP(resp, req)
	if resp.Code == http.StatusProxyAuthRequired {
		t.Fatal("correct CONNECT credentials were rejected")
	}
	if resp.Code != http.StatusNotImplemented {
		t.Fatalf("CONNECT reached unexpected status %d, want 501 without CA setup", resp.Code)
	}
}
