// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

// This file defines the auth command group: login, logout, and status.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"

	"github.com/spf13/cobra"
)

// Connection states reported by `auth status`.
const (
	connectionNotChecked      = "not checked"
	connectionOK              = "ok"
	connectionUnauthorized    = "unauthorized"
	connectionUnauthenticated = "unauthenticated"
)

type authStatusOutput struct {
	APIURL               string `json:"apiUrl"`
	ServiceKeyConfigured bool   `json:"serviceKeyConfigured"`
	Connection           string `json:"connection"`
}

func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage authentication",
	}

	cmd.AddCommand(newAuthLoginCmd())
	cmd.AddCommand(newAuthLogoutCmd())
	cmd.AddCommand(newAuthStatusCmd())

	return cmd
}

func newAuthLoginCmd() *cobra.Command {
	var serviceKey string
	var apiURL string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Store a Fleet Intelligence service key",
		RunE: func(cmd *cobra.Command, _ []string) error {
			serviceKey = strings.TrimSpace(serviceKey)
			if serviceKey == "" {
				return errors.New("service key is required")
			}

			apiURL = strings.TrimSpace(apiURL)
			if apiURL == "" {
				apiURL = config.DefaultAPIURL
			} else if err := validateAPIURL(apiURL); err != nil {
				return err
			}

			if err := config.Save(config.Config{APIURL: apiURL, ServiceKey: serviceKey}); err != nil {
				return err
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Authentication configuration saved.")
			return nil
		},
	}

	cmd.Flags().StringVar(&serviceKey, "key", "", "NGC service key")
	cmd.Flags().StringVar(&apiURL, "api-url", "", "Fleet Intelligence API URL")

	return cmd
}

func newAuthLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove the stored Fleet Intelligence service key",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			cfg.ServiceKey = ""
			if err := config.Save(cfg); err != nil {
				return err
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Authentication service key removed.")
			return nil
		},
	}
}

func newAuthStatusCmd() *cobra.Command {
	common := newCommonFlags()
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show authentication status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			commonFlags := resolveCommonFlags(cmd, common)
			if err := validateReadCommonFlags(commonFlags); err != nil {
				return err
			}

			cfg, err := config.LoadWithEnv()
			if err != nil {
				return err
			}

			status := authStatusOutput{
				APIURL:               cfg.APIURL,
				ServiceKeyConfigured: strings.TrimSpace(cfg.ServiceKey) != "",
				Connection:           connectionNotChecked,
			}
			// Only reach out to the backend when we have both a key and a URL;
			// otherwise the request can't be authenticated.
			if status.ServiceKeyConfigured && strings.TrimSpace(cfg.APIURL) != "" {
				status.Connection = checkConnection(cmd.Context(), commonFlags)
			}
			if commonFlags.output == clioutput.FormatJSON {
				return clioutput.WriteJSON(cmd.OutOrStdout(), status)
			}

			serviceKeyStatus := "not configured"
			if status.ServiceKeyConfigured {
				serviceKeyStatus = "configured"
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "API URL: %s\n", status.APIURL)
			fmt.Fprintf(out, "Service key: %s\n", serviceKeyStatus)
			fmt.Fprintf(out, "Connection: %s\n", status.Connection)

			return nil
		},
	}
	registerReadCommonFlags(cmd, common)
	return cmd
}

// checkConnection verifies the stored credentials against the API's
// /v1/auth/status endpoint and returns a human-readable connection state.
// `auth status` is diagnostic, so transport and auth failures are folded into
// the returned string rather than surfaced as command errors.
func checkConnection(ctx context.Context, common resolvedCommonFlags) string {
	client, err := newConfiguredClient(commonClientOptions(common)...)
	if err != nil {
		return "error: " + err.Error()
	}

	status, err := client.GetAuthStatus(ctx)
	if err != nil {
		var apiErr *nvfleetint.APIError
		if errors.As(err, &apiErr) &&
			(apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden) {
			return connectionUnauthorized
		}
		return "error: " + err.Error()
	}

	if !status.Authenticated {
		return connectionUnauthenticated
	}
	return connectionOK
}

// validateAPIURL rejects an --api-url value before it is written to disk, so a
// bad endpoint fails at login rather than on the next command. The rules live
// in the SDK (nvfleetint.ValidateBaseURL) so the flag, the
// NVFLEETINT_API_URL override, and the stored config are all held to the same
// standard.
func validateAPIURL(rawURL string) error {
	return nvfleetint.ValidateBaseURL(rawURL)
}
