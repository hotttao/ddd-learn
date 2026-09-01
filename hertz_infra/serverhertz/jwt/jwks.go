package jwt

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	jose "github.com/go-jose/go-jose/v4"
)

const maxJWKSSize = 1 << 20

type keySetSource interface {
	Load(context.Context) (jose.JSONWebKeySet, error)
}

type urlKeySetSource struct {
	location string
	client   *http.Client
}

func newURLKeySetSource(location string, client *http.Client) *urlKeySetSource {
	return &urlKeySetSource{location: location, client: client}
}

func (s *urlKeySetSource) Load(ctx context.Context) (jose.JSONWebKeySet, error) {
	u, err := url.Parse(s.location)
	if err != nil {
		return jose.JSONWebKeySet{}, fmt.Errorf("parse JWKS URL: %w", err)
	}

	var body io.ReadCloser
	switch u.Scheme {
	case "http", "https":
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return jose.JSONWebKeySet{}, fmt.Errorf("create JWKS request: %w", err)
		}
		resp, err := s.client.Do(req)
		if err != nil {
			return jose.JSONWebKeySet{}, fmt.Errorf("fetch JWKS: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return jose.JSONWebKeySet{}, fmt.Errorf("fetch JWKS: unexpected HTTP status %d", resp.StatusCode)
		}
		body = resp.Body
	case "file", "":
		path, err := jwksFilePath(u, s.location)
		if err != nil {
			return jose.JSONWebKeySet{}, err
		}
		file, err := os.Open(path)
		if err != nil {
			return jose.JSONWebKeySet{}, fmt.Errorf("open JWKS file: %w", err)
		}
		body = file
	default:
		return jose.JSONWebKeySet{}, fmt.Errorf("unsupported JWKS URL scheme %q", u.Scheme)
	}
	defer body.Close()

	limited := io.LimitReader(body, maxJWKSSize+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return jose.JSONWebKeySet{}, fmt.Errorf("read JWKS: %w", err)
	}
	if len(data) > maxJWKSSize {
		return jose.JSONWebKeySet{}, fmt.Errorf("read JWKS: response exceeds %d bytes", maxJWKSSize)
	}

	var set jose.JSONWebKeySet
	if err := json.Unmarshal(data, &set); err != nil {
		return jose.JSONWebKeySet{}, fmt.Errorf("decode JWKS: %w", err)
	}
	if len(set.Keys) == 0 {
		return jose.JSONWebKeySet{}, fmt.Errorf("decode JWKS: key set is empty")
	}
	for i := range set.Keys {
		key := &set.Keys[i]
		if key.KeyID == "" {
			return jose.JSONWebKeySet{}, fmt.Errorf("decode JWKS: key %d has no kid", i)
		}
		if !key.Valid() || !key.IsPublic() {
			return jose.JSONWebKeySet{}, fmt.Errorf("decode JWKS: key %q is not a valid public key", key.KeyID)
		}
	}
	return set, nil
}

func jwksFilePath(u *url.URL, raw string) (string, error) {
	if u.Scheme == "" {
		return filepath.Clean(raw), nil
	}
	if u.Host != "" && u.Host != "localhost" {
		return "", fmt.Errorf("file JWKS URL cannot contain host %q", u.Host)
	}
	path, err := url.PathUnescape(u.Path)
	if err != nil {
		return "", fmt.Errorf("decode JWKS file path: %w", err)
	}
	if runtime.GOOS == "windows" && len(path) >= 3 && path[0] == '/' && path[2] == ':' {
		path = strings.TrimPrefix(path, "/")
	}
	if path == "" {
		return "", fmt.Errorf("file JWKS URL has no path")
	}
	return filepath.Clean(path), nil
}
