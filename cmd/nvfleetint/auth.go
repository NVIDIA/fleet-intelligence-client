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

	"github.com/NVIDIA/fleet-intelligence-client/internal/clihelpers"
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
	Profile            string `json:"profile"`
	APIURL             string `json:"apiUrl"`
	APIURLSource       string `json:"apiUrlSource"`
	APIKeyConfigured   bool   `json:"apiKeyConfigured"`
	APIKeySource       string `json:"apiKeySource"`
	EnvironmentIgnored bool   `json:"environmentIgnored"`
	Connection         string `json:"connection"`
	// Warnings carries the conditions that credential resolution recovered
	// from. They are part of the answer to "which credentials would a command
	// use", so a script reading this output needs them as much as a person.
	Warnings []string `json:"warnings,omitempty"`
}

type authProfileOutput struct {
	Name             string `json:"name"`
	APIURL           string `json:"apiUrl"`
	APIKeyConfigured bool   `json:"apiKeyConfigured"`
	Current          bool   `json:"current"`
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

Each profile pairs an NGC API key with an API URL, so one installation can
work against several tenants or endpoints. "add" both creates and changes a
profile, so it is also how a key is rotated, and its name is optional — with one
tenant, "nvfleetint auth add --api-key <ngc-api-key>" is the whole setup.

The profile is the object of these commands, so add/remove/use name it
positionally. Commands that call the API instead select a profile with
--profile; without it they use the current profile.`,
		// Without these an unrecognized subcommand — `auth update`, which used to
		// exist — prints the help text and exits 0, so a key-rotation script
		// would report success having changed nothing. Cobra skips Args
		// validation on a command with no RunE, hence both.
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(newAuthAddCmd())
	cmd.AddCommand(newAuthRemoveCmd())
	cmd.AddCommand(newAuthUseCmd())
	cmd.AddCommand(newAuthListCmd())
	cmd.AddCommand(newAuthStatusCmd())

	return cmd
}

// requireProfileNameArg validates the positional <name> these commands act on.
// The profile is what add/remove/use operate on, so it is an argument rather
// than a flag; on API-backed commands --profile means something else entirely
// — which credentials to use (registerProfileFlag).
//
// remove and use require the name: defaulting "delete a profile" or "switch
// profiles" to whichever one happens to be called "default" would act on
// something the user never named. Only add, which is creating the thing, can
// safely supply the name itself.
func requireProfileNameArg() cobra.PositionalArgs {
	return requireSingleArg("profile name")
}

func newAuthAddCmd() *cobra.Command {
	var apiKey, apiURL string
	var skipConfirm bool

	cmd := &cobra.Command{
		Use:   "add [<name>]",
		Short: "Add a credential profile, or change an existing one",
		Long: `Store an NGC API key and API URL under a profile name.

The name is optional: omitting it targets the profile named "` + config.DefaultProfileName + `",
so a single-tenant setup never has to invent one.

An existing profile is changed in place, so this is also the key-rotation path.
The change is partial: an omitted flag leaves that value alone, and rotating a
key therefore preserves a custom API URL.

Replacing a stored API key destroys it, so that prompts for confirmation;
pass --yes to skip the prompt in a script. Creating a profile, changing only its
API URL, or supplying the first key for a profile that has none replaces nothing
recoverable and never prompts.`,
		Example: `  nvfleetint auth add --api-key <ngc-api-key>
  nvfleetint auth add prod --api-key <ngc-api-key>
  nvfleetint auth add dev --api-key <ngc-api-key> --api-url https://dev.example.com
  nvfleetint auth add prod --api-key <rotated-key> --yes
  nvfleetint auth add dev --api-url https://other.example.com`,
		Args: optionalSingleArg("profile name"),
		RunE: func(cmd *cobra.Command, args []string) error {
			// No name means the default profile, so first-time setup is a single
			// command with nothing to name.
			profileArg := config.DefaultProfileName
			if len(args) == 1 {
				profileArg = args[0]
			}
			name, err := validateProfileName(profileArg)
			if err != nil {
				return err
			}

			// Changed() rather than a non-empty check, so an omitted flag is
			// distinguishable from one supplied as empty — which is rejected
			// below rather than treated as "clear this credential", since
			// `--api-key "$KEY"` with KEY unset would otherwise wipe the key.
			inputs := authAddInputs{
				apiKeySet: cmd.Flags().Changed("api-key"),
				apiURLSet: cmd.Flags().Changed("api-url"),
				apiKey:    strings.TrimSpace(apiKey),
			}
			if inputs.apiKeySet && inputs.apiKey == "" {
				return errors.New("--api-key cannot be empty")
			}
			if inputs.apiURLSet && strings.TrimSpace(apiURL) == "" {
				return errors.New("--api-url cannot be empty")
			}
			// Validated before the config file is read or written, so bad input
			// can never leave stored credentials disturbed. Whether a key is
			// *required* depends on the profile existing, so that check has to
			// wait until the config is read.
			if inputs.apiURL, err = resolveNewAPIURL(apiURL); err != nil {
				return err
			}

			// Read once outside the lock to decide whether this overwrites a
			// stored key, so the prompt never blocks other processes on the
			// config lock while it waits for an answer. Any error the plan
			// raises is reported before prompting, so we never ask about a
			// command that was going to fail anyway.
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if _, _, err := resolveAuthAddProfile(cfg, name, inputs); err != nil {
				return err
			}
			confirmed := skipConfirm
			if !confirmed && destroysStoredKey(cfg.Profiles[name], inputs) {
				if err := clihelpers.Confirm(cmd.InOrStdin(), cmd.ErrOrStderr(), overwriteProfileSummary(name, cfg.Profiles[name], inputs)); err != nil {
					return err
				}
				confirmed = true
			}

			var wroteOver, becameCurrent bool
			cfg, err = config.Edit(func(cfg *config.Config) error {
				// Re-planned under the lock: the snapshot the prompt was based
				// on is now stale, and the write has to be correct, not merely
				// consistent with what we showed.
				profile, existedNow, err := resolveAuthAddProfile(*cfg, name, inputs)
				if err != nil {
					return err
				}
				// A key stored since the read — by another process creating the
				// profile, or filling in one that had none — would be destroyed
				// without the confirmation this command promises.
				if !confirmed && destroysStoredKey(cfg.Profiles[name], inputs) {
					return fmt.Errorf(
						"profile %q gained an API key while this command was waiting; re-run to confirm replacing it",
						name,
					)
				}
				wroteOver = existedNow
				wasCurrent := cfg.CurrentProfile == name

				if existedNow {
					if err := cfg.UpdateProfile(name, profile); err != nil {
						return err
					}
				} else if err := cfg.AddProfile(name, profile); err != nil {
					return err
				}
				becameCurrent = cfg.CurrentProfile == name && !wasCurrent

				return nil
			})
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if wroteOver {
				fmt.Fprintf(out, "Profile %q updated.\n", name)
			} else {
				fmt.Fprintf(out, "Profile %q added.\n", name)
			}
			if becameCurrent && cfg.CurrentProfile == name {
				fmt.Fprintf(out, "Profile %q is now the current profile.\n", name)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&apiKey, "api-key", "", "NGC API key")
	cmd.Flags().StringVar(&apiURL, "api-url", "", "Fleet Intelligence API URL")
	cmd.Flags().BoolVar(&skipConfirm, "yes", false,
		"Skip the confirmation prompt shown when a profile is overwritten")

	return cmd
}

// authAddInputs is what `auth add` was told to store. The *Set fields record
// whether a flag was supplied at all, which is what separates "leave this
// alone" from "set it to this".
type authAddInputs struct {
	apiKeySet bool
	apiURLSet bool
	apiKey    string
	apiURL    string
}

// resolveAuthAddProfile decides what `auth add` writes for one snapshot of the
// config, and reports whether the profile already existed. It runs twice per
// command — once on a loaded copy to decide whether to confirm an overwrite,
// then again inside config.Edit so the write is planned against the config as
// it actually is under the lock.
func resolveAuthAddProfile(cfg config.Config, name string, in authAddInputs) (config.Profile, bool, error) {
	profile, lookupErr := cfg.Profile(name)
	existed := lookupErr == nil

	switch {
	case !existed && !in.apiKeySet:
		// A profile with no key cannot authenticate anything.
		return config.Profile{}, false, errors.New("API key is required")
	case existed && !in.apiKeySet && !in.apiURLSet:
		return config.Profile{}, true, fmt.Errorf(
			"profile %q already exists and nothing was supplied to change; pass --api-key, --api-url, or both",
			name,
		)
	}

	if in.apiKeySet {
		profile.APIKey = in.apiKey
	}
	// On an existing profile an omitted --api-url keeps the stored endpoint; on
	// a new one it takes the default.
	if in.apiURLSet || !existed {
		profile.APIURL = in.apiURL
	}

	return profile, existed, nil
}

// destroysStoredKey reports whether `auth add` is about to replace a service
// key that cannot be recovered — the only thing this command destroys, and so
// the only thing worth prompting about. Creating a profile, changing an API
// URL, or supplying the first key for a profile that has none all take nothing
// away, and a prompt that fires when nothing is at stake is one people learn to
// answer without reading. It also has to stay this narrow because the CLI's own
// remediation hints (missingAPIKeyError, fixAPIURLHint) name an existing
// profile: were those to prompt, the printed fix would be unusable in CI.
func destroysStoredKey(current config.Profile, in authAddInputs) bool {
	return in.apiKeySet && strings.TrimSpace(current.APIKey) != ""
}

// overwriteProfileSummary describes the credentials `auth add` is about to
// replace. It names only the fields actually being overwritten — warning about
// an API URL that is not changing would train people to ignore the prompt — and
// never prints a key, only the endpoint it is paired with.
func overwriteProfileSummary(name string, current config.Profile, in authAddInputs) string {
	var replacing []string
	if in.apiKeySet {
		replacing = append(replacing, "API key")
	}
	if in.apiURLSet {
		replacing = append(replacing, "API URL")
	}

	summary := fmt.Sprintf("Profile %q already exists. This replaces its %s.",
		name, strings.Join(replacing, " and "))
	if destroysStoredKey(current, in) {
		if url := strings.TrimSpace(current.APIURL); url != "" {
			summary += fmt.Sprintf("\nThe stored key for %s cannot be recovered.", url)
		} else {
			summary += "\nThe stored key cannot be recovered."
		}
	}

	return summary
}

func newAuthRemoveCmd() *cobra.Command {
	var skipConfirm bool

	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a credential profile",
		Example: `  nvfleetint auth remove dev
  nvfleetint auth remove dev --yes`,
		Args: requireProfileNameArg(),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := validateProfileName(args[0])
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
				summary := fmt.Sprintf("This deletes profile %q and the API key stored in it.", name)
				if err := clihelpers.Confirm(cmd.InOrStdin(), cmd.ErrOrStderr(), summary); err != nil {
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
				fmt.Fprintln(out, "No current profile; run `nvfleetint auth use <name>`.")
			default:
				fmt.Fprintln(out, "No profiles remain; run `nvfleetint auth add <name> --api-key <api-key>`.")
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&skipConfirm, "yes", false, "Skip the confirmation prompt")

	return cmd
}

func newAuthUseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "use <name>",
		Short:   "Select the profile used when --profile is omitted",
		Example: `  nvfleetint auth use prod`,
		Args:    requireProfileNameArg(),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := validateProfileName(args[0])
			if err != nil {
				return err
			}

			if _, err := config.Edit(func(cfg *config.Config) error {
				// "there is nothing to select" is a different problem from
				// "that name is wrong", and needs a different remedy.
				if len(cfg.Profiles) == 0 {
					return fmt.Errorf(
						"%w; run `nvfleetint auth add %s --api-key <api-key>` first",
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
					// API keys are never printed, only reported as present.
					APIKeyConfigured: strings.TrimSpace(profile.APIKey) != "",
					Current:          name == effectiveProfile,
				})
			}

			out := cmd.OutOrStdout()
			if commonFlags.output == clioutput.FormatJSON {
				return clioutput.WriteJSON(out, listOutput)
			}
			if len(listOutput.Profiles) == 0 {
				fmt.Fprintln(out, "No profiles configured. Run `nvfleetint auth add <name> --api-key <api-key>`.")
				return nil
			}

			rows := make([][]string, 0, len(listOutput.Profiles))
			for _, profile := range listOutput.Profiles {
				rows = append(rows, []string{
					profile.Name,
					clioutput.DisplayString(profile.APIURL),
					apiKeyDisplay(profile.APIKeyConfigured),
					currentDisplay(profile.Current),
				})
			}

			if err := clioutput.WriteTable(out, []string{"NAME", "API URL", "API KEY", "ACTIVE"}, rows); err != nil {
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
		// Only an explicit selection still fails; a dangling current_profile
		// resolves and comes back as MissingCurrentProfile below.
		if envProfile != "" {
			return envProfile, fmt.Sprintf(
				"Warning: %s names profile %q, but it is not configured; unset it or choose an existing profile.",
				config.EnvProfile,
				envProfile,
			)
		}
		return "", ""
	}

	if resolved.MissingCurrentProfile != "" {
		return resolved.MissingCurrentProfile, fmt.Sprintf(
			"Warning: current profile %q is not configured; run `nvfleetint auth use <name>`.",
			resolved.MissingCurrentProfile,
		)
	}

	if envProfile != "" {
		return resolved.Profile, fmt.Sprintf("%s selects profile %q for unqualified commands.", config.EnvProfile, resolved.Profile)
	}

	var overrides []string
	if resolved.APIURLSource == config.SourceEnvironment {
		overrides = append(overrides, config.EnvAPIURL)
	}
	if resolved.APIKeySource == config.SourceEnvironment {
		overrides = append(overrides, config.EnvAPIKey)
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
				Profile:            resolved.Profile,
				APIURL:             resolved.APIURL,
				APIURLSource:       string(resolved.APIURLSource),
				APIKeyConfigured:   strings.TrimSpace(resolved.APIKey) != "",
				APIKeySource:       string(resolved.APIKeySource),
				EnvironmentIgnored: len(resolved.EnvIgnored) > 0,
				Connection:         connectionNotChecked,
				Warnings:           credentialWarnings(resolved),
			}
			// Only reach out to the backend when we have both a key and a URL;
			// otherwise the request can't be authenticated.
			if status.APIKeyConfigured && strings.TrimSpace(resolved.APIURL) != "" {
				status.Connection = checkConnection(cmd.Context(), resolved, commonFlags)
			}
			if commonFlags.output == clioutput.FormatJSON {
				return clioutput.WriteJSON(cmd.OutOrStdout(), status)
			}

			apiKeyStatus := "not configured"
			if status.APIKeyConfigured {
				apiKeyStatus = "configured (from " + status.APIKeySource + ")"
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Profile: %s\n", profileDisplay(status.Profile))
			fmt.Fprintf(out, "API URL: %s (from %s)\n", status.APIURL, status.APIURLSource)
			fmt.Fprintf(out, "API key: %s\n", apiKeyStatus)
			fmt.Fprintf(out, "Connection: %s\n", status.Connection)
			for _, warning := range status.Warnings {
				fmt.Fprintf(out, "Warning: %s\n", warning)
			}
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

// validateProfileName trims and checks the <name> argument of an auth command
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
	if resolved.APIKeySource == config.SourceEnvironment {
		shadowing = append(shadowing, config.EnvAPIKey)
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

// apiKeyDisplay renders key presence without ever printing the key
func apiKeyDisplay(configured bool) string {
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
