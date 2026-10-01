// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package ollama

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_Copy_Succeeds(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/copy", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	base, err := url.Parse(server.URL)
	require.NoError(t, err)

	client := NewClient(base, server.Client())
	err = client.Copy(context.Background(), "source-model", "alias-model")
	require.NoError(t, err)
}

func TestClient_Pull_ReportsProgress(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/pull", r.URL.Path)
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte(`{"status":"pulling manifest"}` + "\n"))
	}))
	defer server.Close()

	base, err := url.Parse(server.URL)
	require.NoError(t, err)

	var statuses []string
	client := NewClient(base, server.Client())
	err = client.Pull(context.Background(), "glm-5.3-flash", func(progress ProgressResponse) error {
		statuses = append(statuses, progress.Status)
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"pulling manifest"}, statuses)
}

func TestClient_Pull_ReturnsAPIError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte(`{"error":"model not found"}` + "\n"))
	}))
	defer server.Close()

	base, err := url.Parse(server.URL)
	require.NoError(t, err)

	client := NewClient(base, server.Client())
	err = client.Pull(context.Background(), "missing-model", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "model not found")
}

func TestClient_Copy_ReturnsHTTPError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid copy request"}`))
	}))
	defer server.Close()

	base, err := url.Parse(server.URL)
	require.NoError(t, err)

	client := NewClient(base, server.Client())
	err = client.Copy(context.Background(), "source-model", "alias-model")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid copy request")
}

// newTestClient starts an httptest server running handler and returns a client
// aimed at it. The server is closed when the test finishes.
func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	base, err := url.Parse(server.URL)
	require.NoError(t, err)
	return NewClient(base, server.Client())
}

func TestNewClient_FallsBackToDefaultHTTPClientWhenNoneIsProvided(t *testing.T) {
	t.Parallel()

	base, err := url.Parse("http://localhost:11434")
	require.NoError(t, err)

	client := NewClient(base, nil)

	assert.Same(t, http.DefaultClient, client.http)
	assert.Same(t, base, client.base)
}

func TestNewClient_UsesTheProvidedHTTPClient(t *testing.T) {
	t.Parallel()

	base, err := url.Parse("http://localhost:11434")
	require.NoError(t, err)
	custom := &http.Client{}

	assert.Same(t, custom, NewClient(base, custom).http)
}

func TestClient_Copy_SendsJSONBodyAndNegotiatesJSON(t *testing.T) {
	t.Parallel()

	var (
		gotBody        map[string]string
		gotContentType string
		gotAccept      string
	)
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		gotAccept = r.Header.Get("Accept")
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		w.WriteHeader(http.StatusOK)
	})

	require.NoError(t, client.Copy(context.Background(), "base-model", "alias-model"))

	assert.Equal(t, map[string]string{"source": "base-model", "destination": "alias-model"}, gotBody)
	assert.Equal(t, "application/json", gotContentType)
	assert.Equal(t, "application/json", gotAccept)
}

func TestClient_Pull_SendsExplicitNonStreamingRequestAndNegotiatesNDJSON(t *testing.T) {
	t.Parallel()

	var (
		gotRaw         []byte
		gotContentType string
		gotAccept      string
	)
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		gotAccept = r.Header.Get("Accept")
		var err error
		gotRaw, err = io.ReadAll(r.Body)
		assert.NoError(t, err)
		_, _ = w.Write([]byte(`{"status":"success"}` + "\n"))
	})

	require.NoError(t, client.Pull(context.Background(), "llama3:8b", nil))

	assert.JSONEq(t, `{"model":"llama3:8b","stream":false}`, string(gotRaw), "stream must be sent explicitly because Ollama defaults a missing flag to true")
	assert.Equal(t, "application/json", gotContentType)
	assert.Equal(t, "application/x-ndjson", gotAccept)
}

func TestClient_RequestsAreResolvedUnderTheBaseURLPath(t *testing.T) {
	t.Parallel()

	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		_, _ = w.Write([]byte(`{"status":"success"}` + "\n"))
	}))
	t.Cleanup(server.Close)
	base, err := url.Parse(server.URL + "/proxy/ollama")
	require.NoError(t, err)
	client := NewClient(base, server.Client())

	require.NoError(t, client.Copy(context.Background(), "a", "b"))
	require.NoError(t, client.Pull(context.Background(), "a", nil))

	assert.Equal(t, []string{"/proxy/ollama/api/copy", "/proxy/ollama/api/pull"}, paths)
}

func TestClient_Pull_DeliversEveryProgressEventInOrder(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		for _, status := range []string{"pulling manifest", "downloading", "verifying sha256 digest", "success"} {
			_, _ = w.Write([]byte(`{"status":"` + status + `"}` + "\n"))
		}
	})

	var statuses []string
	err := client.Pull(context.Background(), "llama3", func(progress ProgressResponse) error {
		statuses = append(statuses, progress.Status)
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"pulling manifest", "downloading", "verifying sha256 digest", "success"}, statuses)
}

func TestClient_Pull_AcceptsNilProgressCallback(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"a"}` + "\n" + `{"status":"b"}` + "\n"))
	})

	require.NoError(t, client.Pull(context.Background(), "llama3", nil))
}

func TestClient_Pull_StopsAtTheFirstCallbackError(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"one"}` + "\n" + `{"status":"two"}` + "\n" + `{"status":"three"}` + "\n"))
	})
	errStop := errors.New("stop after first")

	var seen []string
	err := client.Pull(context.Background(), "llama3", func(progress ProgressResponse) error {
		seen = append(seen, progress.Status)
		return errStop
	})

	require.ErrorIs(t, err, errStop)
	assert.Equal(t, []string{"one"}, seen, "events after the failing callback must not be delivered")
}

func TestClient_Pull_ReportsStreamedErrorAfterEarlierProgress(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"pulling manifest"}` + "\n" + `{"error":"disk full"}` + "\n" + `{"status":"never reached"}` + "\n"))
	})

	var seen []string
	err := client.Pull(context.Background(), "llama3", func(progress ProgressResponse) error {
		seen = append(seen, progress.Status)
		return nil
	})

	require.Error(t, err)
	assert.Equal(t, "ollama pull: disk full", err.Error())
	assert.Equal(t, []string{"pulling manifest"}, seen)
}

func TestClient_Pull_RejectsMalformedProgressLine(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("this is not json\n"))
	})

	err := client.Pull(context.Background(), "llama3", nil)

	var syntaxErr *json.SyntaxError
	require.ErrorAs(t, err, &syntaxErr)
}

func TestClient_Pull_ReportsLineLongerThanScannerLimit(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("a", 70*1024)))
	})

	err := client.Pull(context.Background(), "llama3", nil)

	require.ErrorIs(t, err, bufio.ErrTooLong)
}

func TestClient_Pull_MapsHTTPErrorStatusesBeforeReadingTheStream(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{name: "json error body", status: http.StatusNotFound, body: `{"error":"model \"x\" not found"}`, wantErr: `ollama api: model "x" not found`},
		{name: "non-json body falls back to status", status: http.StatusInternalServerError, body: "<html>boom</html>", wantErr: "ollama api: status 500"},
		{name: "empty body falls back to status", status: http.StatusBadGateway, body: "", wantErr: "ollama api: status 502"},
		{name: "json without error field falls back to status", status: http.StatusUnauthorized, body: `{"message":"nope"}`, wantErr: "ollama api: status 401"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var callbackCalls atomic.Int32
			client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			})

			err := client.Pull(context.Background(), "llama3", func(ProgressResponse) error {
				callbackCalls.Add(1)
				return nil
			})

			require.Error(t, err)
			assert.Equal(t, tt.wantErr, err.Error())
			assert.Zero(t, callbackCalls.Load(), "an error response must not be parsed as progress")
		})
	}
}

func TestClient_Copy_MapsHTTPErrorStatuses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{name: "json error body", status: http.StatusNotFound, body: `{"error":"model not found"}`, wantErr: "ollama api: model not found"},
		{name: "non-json body falls back to status", status: http.StatusInternalServerError, body: "oops", wantErr: "ollama api: status 500"},
		{name: "empty error string falls back to status", status: http.StatusConflict, body: `{"error":""}`, wantErr: "ollama api: status 409"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			})

			err := client.Copy(context.Background(), "a", "b")

			require.Error(t, err)
			assert.Equal(t, tt.wantErr, err.Error())
		})
	}
}

func TestClient_Copy_IgnoresBodyOfSuccessfulResponses(t *testing.T) {
	t.Parallel()

	for _, status := range []int{http.StatusOK, http.StatusCreated, http.StatusNoContent} {
		client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
		})

		assert.NoError(t, client.Copy(context.Background(), "a", "b"), "status %d", status)
	}
}

func TestClient_ReturnsContextErrorWhenCallerCancelsBeforeRequest(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler must not run for a canceled context")
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.ErrorIs(t, client.Copy(ctx, "a", "b"), context.Canceled)
	require.ErrorIs(t, client.Pull(ctx, "a", nil), context.Canceled)
}

func TestClient_ReportsTransportFailureWhenServerIsUnreachable(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	base, err := url.Parse(server.URL)
	require.NoError(t, err)
	server.Close()
	client := NewClient(base, &http.Client{Timeout: 2 * time.Second})

	copyErr := client.Copy(context.Background(), "a", "b")
	pullErr := client.Pull(context.Background(), "a", nil)

	var opErr *net.OpError
	require.ErrorAs(t, copyErr, &opErr)
	require.ErrorAs(t, pullErr, &opErr)
}

func TestClient_RejectsUnparseableBaseURLBeforeSendingAnything(t *testing.T) {
	t.Parallel()

	client := NewClient(&url.URL{Scheme: "http", Host: "bad host"}, nil)

	copyErr := client.Copy(context.Background(), "a", "b")
	pullErr := client.Pull(context.Background(), "a", nil)

	var urlErr *url.Error
	require.ErrorAs(t, copyErr, &urlErr)
	assert.Equal(t, "parse", urlErr.Op)
	urlErr = nil
	require.ErrorAs(t, pullErr, &urlErr)
	assert.Equal(t, "parse", urlErr.Op)
}

func TestCheckResponse_OnlyStatusesAtOrAboveBadRequestAreErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{name: "ok", status: http.StatusOK, body: `{"error":"ignored"}`},
		{name: "redirect", status: http.StatusFound},
		{name: "last non-error status", status: 399},
		{name: "first error status without body", status: http.StatusBadRequest, wantErr: "ollama api: status 400"},
		{name: "api error message wins over status", status: http.StatusBadRequest, body: `{"error":"bad model name"}`, wantErr: "ollama api: bad model name"},
		{name: "extra fields are ignored", status: http.StatusTeapot, body: `{"error":"short and stout","code":418}`, wantErr: "ollama api: short and stout"},
		{name: "array body is not an api error", status: http.StatusInternalServerError, body: `["error"]`, wantErr: "ollama api: status 500"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := checkResponse(tt.status, []byte(tt.body))

			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Equal(t, tt.wantErr, err.Error())
		})
	}
}
