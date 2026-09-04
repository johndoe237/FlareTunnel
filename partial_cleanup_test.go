package main

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
)

func TestCleanupWorkersCountDeletesExactlyRequestedWorkers(t *testing.T) {
	deleted, server := cleanupTestServer(t, []string{"flaretunnel-a", "flaretunnel-b", "flaretunnel-c"}, nil)
	defer server.Close()

	client := &CloudflareClient{
		APIToken:  "test-token",
		AccountID: "account-id",
		BaseURL:   server.URL,
		Headers:   map[string]string{"Authorization": "Bearer test-token"},
	}
	ft := &FlareTunnel{Clients: map[string]*CloudflareClient{"main": client}}

	if err := ft.CleanupWorkersCount("main", 2); err != nil {
		t.Fatalf("CleanupWorkersCount() error = %v", err)
	}
	got := deleted.names()
	want := []string{"flaretunnel-a", "flaretunnel-b"}
	if !equalStrings(got, want) {
		t.Fatalf("deleted = %v, want %v", got, want)
	}
}

func TestCleanupWorkersCountNeverDeletesMoreThanExisting(t *testing.T) {
	deleted, server := cleanupTestServer(t, []string{"flaretunnel-a", "flaretunnel-b", "flaretunnel-c"}, nil)
	defer server.Close()

	client := &CloudflareClient{AccountID: "account-id", BaseURL: server.URL}
	ft := &FlareTunnel{Clients: map[string]*CloudflareClient{"main": client}}
	if err := ft.CleanupWorkersCount("main", 10); err != nil {
		t.Fatalf("CleanupWorkersCount() error = %v", err)
	}
	if got := len(deleted.names()); got != 3 {
		t.Fatalf("deleted %d workers, want 3", got)
	}
}

func TestCleanupWorkersCountZeroDoesNothing(t *testing.T) {
	deleted, server := cleanupTestServer(t, []string{"flaretunnel-a"}, nil)
	defer server.Close()

	ft := &FlareTunnel{Clients: map[string]*CloudflareClient{}}
	if err := ft.CleanupWorkersCount("missing", 0); err != nil {
		t.Fatalf("zero cleanup should succeed without account lookup: %v", err)
	}
	if len(deleted.names()) != 0 {
		t.Fatalf("deleted = %v, want no deletions", deleted.names())
	}
}

func TestCleanupWorkersCountListingErrorDoesNotDelete(t *testing.T) {
	deleted, server := cleanupTestServer(t, nil, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		t.Error("unexpected delete request after listing error")
	})
	defer server.Close()

	client := &CloudflareClient{AccountID: "account-id", BaseURL: server.URL}
	ft := &FlareTunnel{Clients: map[string]*CloudflareClient{"main": client}}
	if err := ft.CleanupWorkersCount("main", 1); err == nil {
		t.Fatal("expected listing error")
	}
	if len(deleted.names()) != 0 {
		t.Fatalf("deleted = %v, want no deletions", deleted.names())
	}
}

func TestCleanupWorkersCountContinuesAfterDeleteError(t *testing.T) {
	deleted := &cleanupDeleted{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"result":[{"id":"flaretunnel-a"},{"id":"flaretunnel-b"},{"id":"flaretunnel-c"}]}`))
			return
		}
		if r.Method == http.MethodDelete {
			name := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			deleted.add(name)
			if name == "flaretunnel-b" {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := &CloudflareClient{AccountID: "account-id", BaseURL: server.URL}
	ft := &FlareTunnel{Clients: map[string]*CloudflareClient{"main": client}}
	if err := ft.CleanupWorkersCount("main", 3); err == nil {
		t.Fatal("expected aggregate deletion error")
	}
	got := deleted.names()
	want := []string{"flaretunnel-a", "flaretunnel-b", "flaretunnel-c"}
	if !equalStrings(got, want) {
		t.Fatalf("delete attempts = %v, want %v", got, want)
	}
}

func TestCleanupWorkersCountMissingAccount(t *testing.T) {
	ft := &FlareTunnel{Clients: map[string]*CloudflareClient{}}
	if err := ft.CleanupWorkersCount("missing", 1); err == nil {
		t.Fatal("expected missing account error")
	}
}

type cleanupDeleted struct {
	mu        sync.Mutex
	namesList []string
}

func (d *cleanupDeleted) add(name string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.namesList = append(d.namesList, name)
}

func (d *cleanupDeleted) names() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	result := append([]string(nil), d.namesList...)
	sort.Strings(result)
	return result
}

func cleanupTestServer(t *testing.T, workers []string, override http.HandlerFunc) (*cleanupDeleted, *httptest.Server) {
	t.Helper()
	deleted := &cleanupDeleted{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if override != nil {
			if r.Method == http.MethodDelete {
				name := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
				deleted.add(name)
			}
			before := len(deleted.names())
			override(w, r)
			if len(deleted.names()) != before || r.Method != http.MethodGet {
				return
			}
			if r.Method == http.MethodGet && w.Header().Get("Content-Type") == "" && r.URL.Path != "" {
				return
			}
		}
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"result":[]}`))
			return
		}
		if r.Method == http.MethodDelete {
			name := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			deleted.add(name)
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	if override == nil {
		handler = func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				w.Header().Set("Content-Type", "application/json")
				parts := make([]string, 0, len(workers))
				for _, worker := range workers {
					parts = append(parts, `{"id":"`+worker+`"}`)
				}
				_, _ = w.Write([]byte(`{"result":[` + strings.Join(parts, ",") + `]}`))
				return
			}
			if r.Method == http.MethodDelete {
				name := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
				deleted.add(name)
				w.WriteHeader(http.StatusOK)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}
	}
	return deleted, httptest.NewServer(handler)
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
