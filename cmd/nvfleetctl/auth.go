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

package main

// This file defines the auth command group: login, logout, and status.

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"

	"github.com/spf13/cobra"
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
				Connection:           "not checked",
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
	registerOutputFlag(cmd, common)
	return cmd
}

func validateAPIURL(rawURL string) error {
	// Explicit URLs must be concrete HTTP(S) API roots, not relative paths.
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid API URL: %w", err)
	}
	if !parsedURL.IsAbs() || parsedURL.Host == "" {
		return fmt.Errorf("invalid API URL %q: absolute http or https URL is required", rawURL)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("invalid API URL %q: absolute http or https URL is required", rawURL)
	}

	return nil
}
