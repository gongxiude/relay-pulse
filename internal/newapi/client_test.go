package newapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientListChannels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/channel/" {
			t.Fatalf("path = %s, want /api/channel/", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("Authorization = %q, want %q", got, "Bearer token")
		}
		if got := r.Header.Get("New-Api-User"); got != "user" {
			t.Fatalf("New-Api-User = %q, want %q", got, "user")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"message":"","data":{"items":[{"id":1,"type":1,"status":1,"name":"demo","weight":0,"priority":10,"base_url":"https://example.com","models":"gpt-4o","group":"default","model_mapping":"{\"gpt-4o\":\"gpt-4o\"}","tag":null}],"total":1,"page":1,"page_size":10,"type_counts":{"1":1}}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "token", "user", WithHTTPClient(srv.Client()))
	res, err := c.ListChannels(context.Background())
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("items len = %d, want 1", len(res.Items))
	}
	if res.Items[0].Name != "demo" {
		t.Fatalf("name = %q, want demo", res.Items[0].Name)
	}
}

func TestClientListLogs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/log/" {
			t.Fatalf("path = %s, want /api/log/", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"message":"","data":{"items":[{"id":2,"created_at":1710000000,"type":2,"content":"ok","model_name":"gpt-4o","quota":10,"prompt_tokens":1,"completion_tokens":9,"use_time":3,"is_stream":true,"channel":1,"group":"default","request_id":"r1","upstream_request_id":"u1","other":"{\"frt\":123,\"upstream_model_name\":\"gpt-4o\"}"}],"total":1,"page":1,"page_size":10}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "token", "user", WithHTTPClient(srv.Client()))
	res, err := c.ListLogs(context.Background(), "")
	if err != nil {
		t.Fatalf("ListLogs: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("items len = %d, want 1", len(res.Items))
	}
	if res.Items[0].Type != 2 || len(res.Items[0].Other) == 0 {
		t.Fatalf("unexpected log item: %+v", res.Items[0])
	}
}

func TestClientGetLogStat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/log/stat" {
			t.Fatalf("path = %s, want /api/log/stat", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"message":"","data":{"quota":123,"rpm":4,"tpm":5}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "token", "user", WithHTTPClient(srv.Client()))
	res, err := c.GetLogStat(context.Background())
	if err != nil {
		t.Fatalf("GetLogStat: %v", err)
	}
	if res.Quota != 123 || res.RPM != 4 || res.TPM != 5 {
		t.Fatalf("unexpected stat: %+v", res)
	}
}

func TestClientRejectsBadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "token", "user", WithHTTPClient(srv.Client()), WithTimeout(50*time.Millisecond))
	_, err := c.ListChannels(context.Background())
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("err = %v, want 401 error", err)
	}
}
