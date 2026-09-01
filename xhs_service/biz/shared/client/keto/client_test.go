package keto

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestCheck(t *testing.T) {
	c := &Client{checkEndpoint: "http://keto/relation-tuples/check/openapi", http: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
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
	c := &Client{checkEndpoint: "http://keto/relation-tuples/check/openapi", http: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: ioNopCloser{strings.NewReader("unavailable")}, Header: make(http.Header)}, nil
	})}}
	if _, err := c.Check(context.Background(), "user:alice", "XhsWorkspace", "workspace:demo", "read"); err == nil {
		t.Fatal("expected error")
	}
}

func TestListMembershipsFollowsPaginationAndFiltersRelations(t *testing.T) {
	requests := 0
	c := &Client{relationshipsEndpoint: "http://keto/relation-tuples", http: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		if r.Method != http.MethodGet || r.URL.Query().Get("namespace") != "Organization" || r.URL.Query().Get("subject_id") != "User:alice" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		body := `{"relation_tuples":[{"namespace":"Organization","object":"G","relation":"admins","subject_id":"User:alice"},{"namespace":"Organization","object":"G","relation":"entitled_view_content","subject_id":"User:alice"}],"next_page_token":"next"}`
		if r.URL.Query().Get("page_token") == "next" {
			body = `{"relation_tuples":[{"namespace":"Organization","object":"W","relation":"members","subject_id":"User:alice"}],"next_page_token":""}`
		}
		return &http.Response{StatusCode: http.StatusOK, Body: ioNopCloser{strings.NewReader(body)}, Header: make(http.Header)}, nil
	})}}

	memberships, err := c.ListMemberships(context.Background(), "User:alice")
	if err != nil {
		t.Fatalf("ListMemberships() error = %v", err)
	}
	if requests != 2 || len(memberships) != 2 {
		t.Fatalf("requests = %d, memberships = %#v", requests, memberships)
	}
	if memberships[0].OrganizationID != "G" || memberships[0].Role != "admins" || memberships[1].OrganizationID != "W" {
		t.Fatalf("memberships = %#v", memberships)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type ioNopCloser struct{ *strings.Reader }

func (ioNopCloser) Close() error { return nil }
