// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package console

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// staticFS holds the production build of the console SPA. The source lives in
// console/ at the repository root; `make console-embed` copies its dist/ here.
//
//go:embed static
var staticFS embed.FS

// contentSecurityPolicy confines the console to its own origin. The console
// runs passkey ceremonies, so it loads no third-party script, style, or frame
// and cannot be framed. React style props are applied through the CSSOM and
// are not subject to style-src.
const contentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; " +
	"connect-src 'self'; font-src 'self'; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'"

// Handler returns the HTTP handler serving the embedded Console SPA.
//
// @Summary		Console SPA
// @Description	Serves the g8e Console: passkey authentication, approvals, Operator inventory and binding, cases, investigations, and chat.
// @Tags			public
// @Accept			html
// @Produce		html
// @Success		200	{string}	string	"Returns the index.html SPA"
// @Router			/console/ [get]
func Handler() (http.Handler, error) {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		return nil, fmt.Errorf("console: sub static FS: %w", err)
	}
	files := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setSecurityHeaders(w, r.URL.Path)
		files.ServeHTTP(w, r)
	}), nil
}

func setSecurityHeaders(w http.ResponseWriter, path string) {
	h := w.Header()
	h.Set("Content-Security-Policy", contentSecurityPolicy)
	h.Set("Referrer-Policy", "no-referrer")
	h.Set(constants.HeaderXContentTypeOptions, constants.HeaderValueNoSniff)
	h.Set(constants.HeaderXFrameOptions, constants.HeaderValueDeny)
	h.Set("Permissions-Policy", "camera=(), geolocation=(), microphone=(), payment=(), usb=()")
	if strings.HasPrefix(path, "/assets/") {
		// Vite content-hashes asset filenames, so they never change in place.
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		h.Set("Cache-Control", "no-cache")
	}
}
