// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	gwexplorer "github.com/g8e-ai/g8e/v2/internal/services/gateway/explorer"
)

const evalExplorerRelativeRoot = "dashboard/g8e-adapter/evaluation-explorer/dist"

// NewEvalExplorerHandler serves the evaluation explorer SPA with a runtime.json
// that points at the public mirror origin exposed to browsers.
func NewEvalExplorerHandler(rootOverride, mirrorOrigin string) (http.Handler, error) {
	contentFS, err := resolveEvalExplorerFS(rootOverride)
	if err != nil {
		return nil, err
	}
	if _, err := fs.Stat(contentFS, "index.html"); err != nil {
		return nil, fmt.Errorf("evaluation explorer: missing index.html: %w", err)
	}
	if strings.TrimSpace(mirrorOrigin) == "" {
		mirrorOrigin = fmt.Sprintf("http://127.0.0.1:%d", constants.PublicSpectatorPublicPort)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/runtime.json" || strings.HasSuffix(r.URL.Path, "/runtime.json") {
			writeEvalExplorerRuntime(w, r, mirrorOrigin)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			serveEvalExplorerFile(w, r, contentFS, "index.html")
			return
		}
		if strings.Contains(path, "..") {
			http.NotFound(w, r)
			return
		}
		if _, err := fs.Stat(contentFS, path); err == nil {
			serveEvalExplorerFile(w, r, contentFS, path)
			return
		}
		serveEvalExplorerFile(w, r, contentFS, "index.html")
	}), nil
}

func serveEvalExplorerFile(w http.ResponseWriter, r *http.Request, contentFS fs.FS, name string) {
	http.ServeFileFS(w, r, contentFS, name)
}

// combinePublicSpectatorHandler serves anonymous mirror reads and the evaluation
// explorer SPA from one public origin.
func combinePublicSpectatorHandler(mirrorPublic, explorer http.Handler) http.Handler {
	if mirrorPublic == nil {
		return explorer
	}
	if explorer == nil {
		return mirrorPublic
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if publicMirrorAnonymousReadPath(r.URL.Path) {
			mirrorPublic.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		explorer.ServeHTTP(w, r)
	})
}

func resolveEvalExplorerMirrorOrigin(publicBaseURL, publicListenAddress string) string {
	if origin := strings.TrimSpace(publicBaseURL); origin != "" {
		return strings.TrimRight(origin, "/")
	}
	return publicMirrorURL(publicListenAddress)
}

func resolveEvalExplorerFS(rootOverride string) (fs.FS, error) {
	if root := strings.TrimSpace(rootOverride); root != "" {
		if info, err := os.Stat(root); err == nil && info.IsDir() {
			return os.DirFS(root), nil
		}
	}
	if envRoot := strings.TrimSpace(os.Getenv("G8E_EVAL_EXPLORER_ROOT")); envRoot != "" {
		if info, err := os.Stat(envRoot); err == nil && info.IsDir() {
			return os.DirFS(envRoot), nil
		}
	}
	if embedded, err := gwexplorer.StaticFS(); err == nil {
		if _, err := fs.Stat(embedded, "index.html"); err == nil {
			return embedded, nil
		}
	}
	for _, candidate := range evalExplorerDiskCandidates() {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return os.DirFS(candidate), nil
		}
	}
	return nil, fmt.Errorf("evaluation explorer: dist directory not found")
}

func evalExplorerDiskCandidates() []string {
	candidates := []string{
		evalExplorerRelativeRoot,
		filepath.Join("..", evalExplorerRelativeRoot),
	}
	if cwd, err := os.Getwd(); err == nil {
		for depth := 0; depth < 5; depth++ {
			candidates = append(candidates, filepath.Join(cwd, evalExplorerRelativeRoot))
			parent := filepath.Dir(cwd)
			if parent == cwd {
				break
			}
			cwd = parent
		}
	}
	return candidates
}

func publicMirrorURL(listenAddress string) string {
	if strings.TrimSpace(listenAddress) == "" {
		return fmt.Sprintf("http://127.0.0.1:%d", constants.PublicSpectatorPublicPort)
	}
	if strings.HasPrefix(listenAddress, "http://") || strings.HasPrefix(listenAddress, "https://") {
		return listenAddress
	}
	host, port, err := net.SplitHostPort(listenAddress)
	if err != nil {
		return "http://" + listenAddress
	}
	if host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("http://%s:%s", host, port)
}

func writeEvalExplorerRuntime(w http.ResponseWriter, r *http.Request, configuredOrigin string) {
	payload := map[string]string{
		"schema_version": "1.0.0",
		"mirror_origin":  resolveRequestMirrorOrigin(r, configuredOrigin),
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

// resolveRequestMirrorOrigin returns the mirror API origin the browser should
// call. Loopback and same-host requests use the request origin so local
// http://127.0.0.1:8082 works while https://opendevops.ai still resolves
// through the tunnel with the configured public base URL.
func resolveRequestMirrorOrigin(r *http.Request, configuredOrigin string) string {
	configuredOrigin = strings.TrimRight(strings.TrimSpace(configuredOrigin), "/")
	if r == nil {
		return fallbackMirrorOrigin(configuredOrigin)
	}
	requestOrigin := requestOriginURL(r)
	if isLoopbackRequestHost(r.Host) {
		return requestOrigin
	}
	if configuredOrigin != "" {
		if configuredHost := publicURLHost(configuredOrigin); configuredHost != "" {
			if requestHost := requestHostname(r.Host); strings.EqualFold(requestHost, configuredHost) {
				return configuredOrigin
			}
		}
	}
	return requestOrigin
}

func fallbackMirrorOrigin(configuredOrigin string) string {
	if configuredOrigin != "" {
		return configuredOrigin
	}
	return fmt.Sprintf("http://127.0.0.1:%d", constants.PublicSpectatorPublicPort)
}

func requestOriginURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); forwarded != "" {
		scheme = strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}
	host := strings.TrimSpace(r.Host)
	if forwardedHost := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); forwardedHost != "" {
		host = strings.TrimSpace(strings.Split(forwardedHost, ",")[0])
	}
	return fmt.Sprintf("%s://%s", scheme, host)
}

func requestHostname(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	hostname, _, err := net.SplitHostPort(host)
	if err != nil {
		return host
	}
	return hostname
}

func isLoopbackRequestHost(host string) bool {
	switch strings.ToLower(requestHostname(host)) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return strings.HasPrefix(strings.ToLower(requestHostname(host)), "127.")
	}
}

func publicURLHost(origin string) string {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return ""
	}
	return requestHostname(parsed.Host)
}
