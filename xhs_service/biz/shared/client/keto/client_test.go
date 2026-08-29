package keto

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestCheck(t *testing.T) {
	c := &Client{endpoint: "http://keto/relation-tuples/check/openapi", http: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/relation-tuples/check/openapi") {
			t.Fatalf("unexpected request")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: ioNopCloser{strings.NewReader(`{"allowed":true}`)}, Header: make(http.Header)}, nil
	})}}
	ok, err := c.Check(context.Background(), "user:alice", "XhsWorkspace", "workspace:demo", "read")
	if err != nil || !ok {
		t.Fatalf("check = %v, %v", ok, err)
	}
}

func TestCheckFailsClosedOnServerError(t *testing.T) {
	c := &Client{endpoint: "http://keto/relation-tuples/check/openapi", http: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: ioNopCloser{strings.NewReader("unavailable")}, Header: make(http.Header)}, nil
	})}}
	if _, err := c.Check(context.Background(), "user:alice", "XhsWorkspace", "workspace:demo", "read"); err == nil {
		t.Fatal("expected error")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type ioNopCloser struct{ *strings.Reader }

func (ioNopCloser) Close() error { return nil }
