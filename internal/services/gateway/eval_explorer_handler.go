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
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

const evalExplorerRelativeRoot = "dashboard/g8e-adapter/evaluation-explorer/dist"

// NewEvalExplorerHandler serves the built evaluation explorer SPA with a
// runtime.json that points at the gateway-owned public mirror listener.
func NewEvalExplorerHandler(rootOverride, publicMirrorListenAddress string) (http.Handler, error) {
	root := resolveEvalExplorerRoot(rootOverride)
	if root == "" {
		return nil, fmt.Errorf("evaluation explorer: dist directory not found")
	}
	indexPath := filepath.Join(root, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		return nil, fmt.Errorf("evaluation explorer: missing index.html: %w", err)
	}
	publicMirrorOrigin := publicMirrorURL(publicMirrorListenAddress)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/runtime.json" || strings.HasSuffix(r.URL.Path, "/runtime.json") {
			writeEvalExplorerRuntime(w, publicMirrorOrigin)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		clean := filepath.Clean(path)
		if clean == "." || strings.HasPrefix(clean, "..") {
			http.NotFound(w, r)
			return
		}
		fullPath := filepath.Join(root, clean)
		if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
			http.ServeFile(w, r, fullPath)
			return
		}
		http.ServeFile(w, r, indexPath)
	}), nil
}

func resolveEvalExplorerRoot(rootOverride string) string {
	if strings.TrimSpace(rootOverride) != "" {
		if info, err := os.Stat(rootOverride); err == nil && info.IsDir() {
			return rootOverride
		}
	}
	if envRoot := strings.TrimSpace(os.Getenv("G8E_EVAL_EXPLORER_ROOT")); envRoot != "" {
		if info, err := os.Stat(envRoot); err == nil && info.IsDir() {
			return envRoot
		}
	}
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
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	return ""
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

func writeEvalExplorerRuntime(w http.ResponseWriter, mirrorOrigin string) {
	payload := map[string]string{
		"schema_version": "1.0.0",
		"mirror_origin":  mirrorOrigin,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}
