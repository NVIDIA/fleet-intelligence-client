// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

// This file defines the auth command group: credential profile CRUD
// (add/update/remove/use/list) plus the diagnostic status command.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
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
	Profile              string `json:"profile"`
	APIURL               string `json:"apiUrl"`
	APIURLSource         string `json:"apiUrlSource"`
	ServiceKeyConfigured bool   `json:"serviceKeyConfigured"`
	ServiceKeySource     string `json:"serviceKeySource"`
	EnvironmentIgnored   bool   `json:"environmentIgnored"`
	Connection           string `json:"connection"`
}

type authProfileOutput struct {
	Name                 string `json:"name"`
	APIURL               string `json:"apiUrl"`
	ServiceKeyConfigured bool   `json:"serviceKeyConfigured"`
	Current              bool   `json:"current"`
}

type authListOutput struct {
	CurrentProfile string              `json:"currentProfile"`
	Profiles       []authProfileOutput `json:"profiles"`
}

func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage credential profiles",
		Long: `Manage the credential profiles nvfleetint uses to reach the API.

Each profile pairs an NGC service key with an API URL, so one installation can
work against several tenants or endpoints. Commands that call the API select a
profile with --profile; without it they use the current profile.`,
	}

	cmd.AddCommand(newAuthAddCmd())
	cmd.AddCommand(newAuthUpdateCmd())
	cmd.AddCommand(newAuthRemoveCmd())
	cmd.AddCommand(newAuthUseCmd())
	cmd.AddCommand(newAuthListCmd())
	cmd.AddCommand(newAuthStatusCmd())

	return cmd
}

// Registers --profile as the name of the profile being changed. On API-backed
// commands the same flag instead selects credentials (registerProfileFlag).
func registerProfileNameFlag(cmd *cobra.Command, name *string) {
	cmd.Flags().StringVar(name, "profile", "", "Profile name")
}

func newAuthAddCmd() *cobra.Command {
	var profileName, serviceKey, apiURL string

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a credential profile",
		Example: `  nvfleetint auth add --profile prod --key <ngc-service-key>
  nvfleetint auth add --profile dev --key <ngc-service-key> --api-url https://dev.example.com`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			name, err := validateProfileName(profileName)
			if err != nil {
				return err
			}

			serviceKey = strings.TrimSpace(serviceKey)
			if serviceKey == "" {
				return errors.New("service key is required")
			}
			apiURL, err = resolveNewAPIURL(apiURL)
			if err != nil {
				return err
			}

			cfg, err := config.Edit(func(cfg *config.Config) error {
				return cfg.AddProfile(name, config.Profile{APIURL: apiURL, ServiceKey: serviceKey})
			})
			if err != nil {
				if errors.Is(err, config.ErrProfileExists) {
					return fmt.Errorf("%w; run `nvfleetint auth update --profile %s` to change it", err, name)
				}
				return err
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Profile %q added.\n", name)
			if cfg.CurrentProfile == name {
				fmt.Fprintf(out, "Profile %q is now the current profile.\n", name)
			}

			return nil
		},
	}

	registerProfileNameFlag(cmd, &profileName)
	cmd.Flags().StringVar(&serviceKey, "key", "", "NGC service key")
	cmd.Flags().StringVar(&apiURL, "api-url", "", "Fleet Intelligence API URL")

	return cmd
}

func newAuthUpdateCmd() *cobra.Command {
	var profileName, serviceKey, apiURL string

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Change the credentials stored in a profile",
		Example: `  nvfleetint auth update --profile prod --key <ngc-service-key>
  nvfleetint auth update --profile dev --api-url https://dev.example.com`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			name, err := validateProfileName(profileName)
			if err != nil {
				return err
			}

			// Changed() rather than a non-empty check, so an omitted flag is
			// distinguishable from one supplied as empty — which is rejected
			// below rather than treated as "clear this credential", since
			// `--key "$KEY"` with KEY unset would otherwise wipe the key.
			keySet := cmd.Flags().Changed("key")
			apiURLSet := cmd.Flags().Changed("api-url")
			if !keySet && !apiURLSet {
				return errors.New("nothing to update: pass --key, --api-url, or both")
			}
			if keySet && strings.TrimSpace(serviceKey) == "" {
				return errors.New("--key cannot be empty")
			}
			if apiURLSet && strings.TrimSpace(apiURL) == "" {
				return errors.New("--api-url cannot be empty")
			}

			if _, err := config.Edit(func(cfg *config.Config) error {
				profile, err := cfg.Profile(name)
				if err != nil {
					return withProfileListHint(err)
				}

				if keySet {
					profile.ServiceKey = strings.TrimSpace(serviceKey)
				}
				if apiURLSet {
					if profile.APIURL, err = resolveNewAPIURL(apiURL); err != nil {
						return err
					}
				}

				return cfg.UpdateProfile(name, profile)
			}); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Profile %q updated.\n", name)

			return nil
		},
	}

	registerProfileNameFlag(cmd, &profileName)
	cmd.Flags().StringVar(&serviceKey, "key", "", "NGC service key")
	cmd.Flags().StringVar(&apiURL, "api-url", "", "Fleet Intelligence API URL")

	return cmd
}

func newAuthRemoveCmd() *cobra.Command {
	var profileName string
	var skipConfirm bool

	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove a credential profile",
		Example: `  nvfleetint auth remove --profile dev
  nvfleetint auth remove --profile dev --yes`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			name, err := validateProfileName(profileName)
			if err != nil {
				return err
			}

			cfg, err := config.Load()
			if err != nil {
				return err
			}
			// Confirm before the delete, but only once the profile is known to
			// exist — prompting about a typo would be noise.
			if _, err := cfg.Profile(name); err != nil {
				return withProfileListHint(err)
			}
			if !skipConfirm {
				summary := fmt.Sprintf("This deletes profile %q and the service key stored in it.", name)
				if err := confirm(cmd, summary); err != nil {
					return err
				}
			}

			cfg, err = config.Edit(func(cfg *config.Config) error {
				return cfg.RemoveProfile(name)
			})
			if err != nil {
				if errors.Is(err, config.ErrProfileNotFound) {
					return withProfileListHint(err)
				}
				return err
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Profile %q removed.\n", name)
			switch {
			case cfg.CurrentProfile != "":
				fmt.Fprintf(out, "Current profile: %s\n", cfg.CurrentProfile)
			case len(cfg.Profiles) > 0:
				fmt.Fprintln(out, "No current profile; run `nvfleetint auth use --profile <name>`.")
			default:
				fmt.Fprintln(out, "No profiles remain; run `nvfleetint auth add --profile <name> --key <service-key>`.")
			}

			return nil
		},
	}

	registerProfileNameFlag(cmd, &profileName)
	cmd.Flags().BoolVar(&skipConfirm, "yes", false, "Skip the confirmation prompt")

	return cmd
}

func newAuthUseCmd() *cobra.Command {
	var profileName string

	cmd := &cobra.Command{
		Use:     "use",
		Short:   "Select the profile used when --profile is omitted",
		Example: `  nvfleetint auth use --profile prod`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			name, err := validateProfileName(profileName)
			if err != nil {
				return err
			}

			if _, err := config.Edit(func(cfg *config.Config) error {
				// "there is nothing to select" is a different problem from
				// "that name is wrong", and needs a different remedy.
				if len(cfg.Profiles) == 0 {
					return fmt.Errorf(
						"%w; run `nvfleetint auth add --profile %s --key <service-key>` first",
						config.ErrNoProfile, name,
					)
				}
				return cfg.UseProfile(name)
			}); err != nil {
				if errors.Is(err, config.ErrProfileNotFound) {
					return withProfileListHint(err)
				}
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Current profile: %s\n", name)

			return nil
		},
	}

	registerProfileNameFlag(cmd, &profileName)

	return cmd
}

func newAuthListCmd() *cobra.Command {
	common := newCommonFlags()

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List credential profiles",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			commonFlags := resolveCommonFlags(cmd, common)
			if err := validateReadCommonFlags(commonFlags); err != nil {
				return err
			}

			cfg, err := config.Load()
			if err != nil {
				return err
			}

			effectiveProfile, listNote := authListEffectiveProfile(cfg)
			listOutput := authListOutput{
				CurrentProfile: effectiveProfile,
				Profiles:       make([]authProfileOutput, 0, len(cfg.Profiles)),
			}
			for _, name := range cfg.ProfileNames() {
				profile := cfg.Profiles[name]
				listOutput.Profiles = append(listOutput.Profiles, authProfileOutput{
					Name:   name,
					APIURL: profile.APIURL,
					// Service keys are never printed, only reported as present.
					ServiceKeyConfigured: strings.TrimSpace(profile.ServiceKey) != "",
					Current:              name == effectiveProfile,
				})
			}

			out := cmd.OutOrStdout()
			if commonFlags.output == clioutput.FormatJSON {
				return clioutput.WriteJSON(out, listOutput)
			}
			if len(listOutput.Profiles) == 0 {
				fmt.Fprintln(out, "No profiles configured. Run `nvfleetint auth add --profile <name> --key <service-key>`.")
				return nil
			}

			rows := make([][]string, 0, len(listOutput.Profiles))
			for _, profile := range listOutput.Profiles {
				rows = append(rows, []string{
					profile.Name,
					clioutput.DisplayString(profile.APIURL),
					serviceKeyDisplay(profile.ServiceKeyConfigured),
					currentDisplay(profile.Current),
				})
			}

			if err := clioutput.WriteTable(out, []string{"NAME", "API URL", "SERVICE KEY", "ACTIVE"}, rows); err != nil {
				return err
			}
			if listNote != "" {
				fmt.Fprintln(out, listNote)
			}
			return nil
		},
	}

	// No --profile here: this command lists every profile rather than using one.
	registerOutputFlag(cmd, common)

	return cmd
}

func authListEffectiveProfile(cfg config.Config) (string, string) {
	envProfile := strings.TrimSpace(os.Getenv(config.EnvProfile))
	resolved, err := cfg.Resolve("")
	if err != nil {
		if envProfile != "" {
			return envProfile, fmt.Sprintf(
				"Warning: %s names profile %q, but it is not configured; unset it or choose an existing profile.",
				config.EnvProfile,
				envProfile,
			)
		}
		if current := strings.TrimSpace(cfg.CurrentProfile); current != "" {
			return current, fmt.Sprintf(
				"Warning: current profile %q is not configured; run `nvfleetint auth use --profile <name>`.",
				current,
			)
		}
		return "", ""
	}

	if envProfile != "" {
		return resolved.Profile, fmt.Sprintf("%s selects profile %q for unqualified commands.", config.EnvProfile, resolved.Profile)
	}

	var overrides []string
	if resolved.APIURLSource == config.SourceEnvironment {
		overrides = append(overrides, config.EnvAPIURL)
	}
	if resolved.ServiceKeySource == config.SourceEnvironment {
		overrides = append(overrides, config.EnvServiceKey)
	}
	if len(overrides) > 0 && resolved.Profile != "" {
		return resolved.Profile, fmt.Sprintf(
			"%s %s overriding values stored in profile %q; run `nvfleetint auth status` for effective credentials.",
			strings.Join(overrides, " / "),
			isAre(len(overrides)),
			resolved.Profile,
		)
	}

	return resolved.Profile, ""
}

func newAuthStatusCmd() *cobra.Command {
	common := newCommonFlags()

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show which credentials a command would use, and whether they work",
		Example: `  nvfleetint auth status
  nvfleetint auth status --profile dev`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			commonFlags := resolveCommonFlags(cmd, common)
			if err := validateReadCommonFlags(commonFlags); err != nil {
				return err
			}

			resolved, err := resolveCredentials(commonFlags.profile)
			if err != nil {
				return err
			}

			status := authStatusOutput{
				Profile:              resolved.Profile,
				APIURL:               resolved.APIURL,
				APIURLSource:         string(resolved.APIURLSource),
				ServiceKeyConfigured: strings.TrimSpace(resolved.ServiceKey) != "",
				ServiceKeySource:     string(resolved.ServiceKeySource),
				EnvironmentIgnored:   len(resolved.EnvIgnored) > 0,
				Connection:           connectionNotChecked,
			}
			// Only reach out to the backend when we have both a key and a URL;
			// otherwise the request can't be authenticated.
			if status.ServiceKeyConfigured && strings.TrimSpace(resolved.APIURL) != "" {
				status.Connection = checkConnection(cmd.Context(), resolved, commonFlags)
			}
			if commonFlags.output == clioutput.FormatJSON {
				return clioutput.WriteJSON(cmd.OutOrStdout(), status)
			}

			serviceKeyStatus := "not configured"
			if status.ServiceKeyConfigured {
				serviceKeyStatus = "configured (from " + status.ServiceKeySource + ")"
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Profile: %s\n", profileDisplay(status.Profile))
			fmt.Fprintf(out, "API URL: %s (from %s)\n", status.APIURL, status.APIURLSource)
			fmt.Fprintf(out, "Service key: %s\n", serviceKeyStatus)
			fmt.Fprintf(out, "Connection: %s\n", status.Connection)
			if len(resolved.EnvIgnored) > 0 {
				fmt.Fprintf(out, "Note: %s %s set but ignored because a profile was selected explicitly.\n",
					strings.Join(resolved.EnvIgnored, " / "), isAre(len(resolved.EnvIgnored)))
			}
			if note := shadowedProfileNote(resolved); note != "" {
				fmt.Fprintln(out, note)
			}

			return nil
		},
	}

	registerReadCommonFlags(cmd, common)

	return cmd
}

// checkConnection verifies the resolved credentials against the API's
// /v1/auth/status endpoint and returns a human-readable connection state.
// `auth status` is diagnostic, so transport and auth failures are folded into
// the returned string rather than surfaced as command errors.
func checkConnection(ctx context.Context, resolved config.Resolved, common resolvedCommonFlags) string {
	client, err := clientFromResolved(resolved, common)
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

// validateProfileName trims and checks the --profile value of an auth command
func validateProfileName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if err := config.ValidateProfileName(name); err != nil {
		return "", err
	}

	return name, nil
}

// withProfileListHint points the user at `auth list` when a profile is missing
func withProfileListHint(err error) error {
	if errors.Is(err, config.ErrProfileNotFound) {
		return fmt.Errorf("%w; run `nvfleetint auth list` to see the configured profiles", err)
	}

	return err
}

// resolveNewAPIURL validates an --api-url value before it is written to disk,
// so a bad endpoint fails now rather than on the next command. The rules live
// in the SDK (nvfleetint.ValidateBaseURL) so the flag, the NVFLEETINT_API_URL
// override, and the stored profile are all held to the same standard.
func resolveNewAPIURL(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return config.DefaultAPIURL, nil
	}
	if err := nvfleetint.ValidateBaseURL(rawURL); err != nil {
		return "", err
	}

	return rawURL, nil
}

// shadowedProfileNote warns when an environment variable is overriding part of
// the profile that was actually selected. That only happens without an explicit
// --profile; naming the profile is what makes the surprise diagnosable.
func shadowedProfileNote(resolved config.Resolved) string {
	if resolved.Profile == "" {
		return ""
	}

	var shadowing []string
	if resolved.APIURLSource == config.SourceEnvironment {
		shadowing = append(shadowing, config.EnvAPIURL)
	}
	if resolved.ServiceKeySource == config.SourceEnvironment {
		shadowing = append(shadowing, config.EnvServiceKey)
	}
	if len(shadowing) == 0 {
		return ""
	}

	verb, noun := "overrides", "value"
	if len(shadowing) > 1 {
		verb, noun = "override", "values"
	}

	return fmt.Sprintf("Note: %s %s the %s stored in profile %q.",
		strings.Join(shadowing, " / "), verb, noun, resolved.Profile)
}

// isAre picks the verb form agreeing with how many items a note lists
func isAre(count int) string {
	if count == 1 {
		return "is"
	}

	return "are"
}

// profileDisplay names the profile in use, or the reserved token when none is
func profileDisplay(name string) string {
	if strings.TrimSpace(name) == "" {
		return config.ReservedProfileName
	}

	return name
}

// serviceKeyDisplay renders key presence without ever printing the key
func serviceKeyDisplay(configured bool) string {
	if configured {
		return "configured"
	}

	return "not configured"
}

// currentDisplay marks the profile used when --profile is omitted
func currentDisplay(current bool) string {
	if current {
		return "*"
	}

	return "-"
}
