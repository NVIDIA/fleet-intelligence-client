// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package fleetintelligence

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/NVIDIA/fleet-intelligence-client/internal/generated/fleetapi"
)

// AuthStatus reports whether the configured credentials authenticate against
// the API.
type AuthStatus struct {
	// Authenticated is true when the request reached the backend through the
	// authenticated path.
	Authenticated bool   `json:"authenticated"`
	RawJSON       []byte `json:"-"`
}

// GetAuthStatus verifies the configured service key against the API by calling
// GET /v1/auth/status. A non-2xx response is returned as an *APIError so
// callers can distinguish rejected credentials (401/403) from other failures.
func (c *Client) GetAuthStatus(ctx context.Context) (AuthStatus, error) {
	ctx, cancel := c.requestContext(ctx)
	defer cancel()

	resp, err := c.api.GetV1AuthStatusWithResponse(ctx)
	if err != nil {
		return AuthStatus{}, err
	}
	if resp.StatusCode() != http.StatusOK {
		return AuthStatus{}, newAPIError(resp.StatusCode(), resp.Status(), resp.Body)
	}

	var data fleetapi.ModelsAuthStatusResponse
	if err := json.Unmarshal(resp.Body, &data); err != nil {
		return AuthStatus{}, err
	}

	return AuthStatus{
		Authenticated: boolValue(data.Authenticated),
		RawJSON:       append([]byte(nil), resp.Body...),
	}, nil
}
