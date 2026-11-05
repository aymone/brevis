package kubernetes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestListPodsFollowsPagesAndReadsOnlyItsNamespace(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer tok" || r.URL.Query().Get("fieldSelector") != "status.phase=Running" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if r.URL.Query().Get("continue") == "" {
			_, _ = w.Write([]byte(`{"metadata":{"continue":"c1"},"items":[{"metadata":{"name":"a"},"status":{"podIP":"10.0.0.1"}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"items":[{"metadata":{"name":"b"}}]}`))
	}))
	defer srv.Close()
	c := &Client{base: srv.URL, namespace: "brevis", http: srv.Client(), token: "tok", tokenRead: time.Now(), tokenTTL: 1 << 60}
	pods, err := c.ListPods(context.Background())
	if err != nil || len(pods) != 2 || pods[0].Status.PodIP != "10.0.0.1" {
		t.Fatalf("%v %v", pods, err)
	}
	for _, p := range paths {
		if p != "/api/v1/namespaces/brevis/pods" {
			t.Errorf("asked for %s", p)
		}
	}
}

func TestListPodsReportsTheServersReason(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"pods is forbidden"}`))
	}))
	defer srv.Close()
	c := &Client{base: srv.URL, namespace: "brevis", http: srv.Client(), token: "tok", tokenRead: time.Now(), tokenTTL: 1 << 60}
	if _, err := c.ListPods(context.Background()); err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("%v", err)
	}
}
