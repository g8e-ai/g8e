// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package console

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static
var staticFS embed.FS

// Handler returns the HTTP handler serving the embedded Console SPA.
//
// @Summary		Console SPA
// @Description	Serves the single-page application dashboard for WebAuthn/passkey operations.
// @Tags			public
// @Accept			html
// @Produce		html
// @Success		200	{string}	string	"Returns the index.html SPA"
// @Router			/console/ [get]
func Handler() http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic("console: failed to sub static FS: " + err.Error())
	}
	return http.FileServer(http.FS(sub))
}
