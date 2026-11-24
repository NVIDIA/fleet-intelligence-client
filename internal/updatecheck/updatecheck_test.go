// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package updatecheck

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Serves the latest-release permalink the way github.com does: a redirect to
// the release's own page. observe, when set, sees every request the server gets,
// including a followed redirect the checker was supposed to stop at.
func redirectServer(t *testing.T, tag string, observe func(*http.Request)) *httptest.Server {
	t.Helper()

	var base string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if observe != nil {
			observe(r)
		}
		if strings.Contains(r.URL.Path, releaseTagSegment) {
			_, _ = w.Write([]byte("release page"))
			return
		}
		w.Header().Set("Location", base+releaseTagSegment+tag)
		w.WriteHeader(http.StatusFound)
	}))
	base = server.URL
	t.Cleanup(server.Close)
	return server
}
