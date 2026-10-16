package voice

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPosition(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Internal-IPC-Token") != "tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Query().Get("guildId") {
		case "playing":
			_, _ = w.Write([]byte(`{"active":true,"playing":true,"positionMs":61540}`))
		default:
			_, _ = w.Write([]byte(`{"active":false,"positionMs":0}`))
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "tok")
	if pos, ok := c.Position("playing"); !ok || pos != 61540*time.Millisecond {
		t.Fatalf("Position = %v, %v; want 61.54s, true", pos, ok)
	}
	if _, ok := c.Position("idle"); ok {
		t.Fatal("an idle guild should report ok=false")
	}
	if _, ok := NewClient(srv.URL, "wrong").Position("playing"); ok {
		t.Fatal("a rejected request should report ok=false")
	}
}
