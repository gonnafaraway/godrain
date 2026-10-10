package load

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPFetcherPlainFloat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("42.5\n"))
	}))
	t.Cleanup(srv.Close)

	f := &HTTPFetcher{URL: srv.URL}
	v, err := f.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if v != 42.5 {
		t.Fatalf("got %v", v)
	}
}

func TestHTTPFetcherPrometheus(t *testing.T) {
	body := `# HELP http_requests_total count
# TYPE http_requests_total counter
http_requests_total{code="200"} 100
other_metric 1
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	f := &HTTPFetcher{URL: srv.URL, MetricName: "http_requests_total"}
	v, err := f.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if v != 100 {
		t.Fatalf("got %v", v)
	}
}

func TestSampleOnce(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("7"))
	}))
	t.Cleanup(srv.Close)

	lp, err := SampleOnce(context.Background(), &HTTPFetcher{URL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	if lp.Value != 7 {
		t.Fatalf("got %v", lp.Value)
	}
	if lp.Time.IsZero() {
		t.Fatal("expected timestamp")
	}
}
