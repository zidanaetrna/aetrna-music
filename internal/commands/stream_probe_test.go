package commands

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProbeStreamURL(t *testing.T) {
	var gotRange, gotUA string
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRange, gotUA = r.Header.Get("Range"), r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte{0})
	}))
	defer ok.Close()
	forbidden := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer forbidden.Close()

	if err := ProbeStreamURL(ok.URL); err != nil {
		t.Fatalf("206 should pass the probe: %v", err)
	}
	if gotRange != "bytes=0-0" || gotUA != streamUserAgent {
		t.Errorf("probe should fetch one byte with ffmpeg's User-Agent, got Range=%q UA=%q", gotRange, gotUA)
	}
	if err := ProbeStreamURL(forbidden.URL); err == nil {
		t.Fatal("403 must fail the probe")
	}
}
