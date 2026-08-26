// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package auth

// This file defines the auth command group: credential profile CRUD
// (add/update/remove/use/list) plus the diagnostic status command.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdutil"
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

func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage credential profiles",
		Long: `Manage the credential profiles nvfleetint uses to reach the API.

Each profile pairs an NGC API key with an API URL, so one installation can
work against several tenants or endpoints. "add" both creates and changes a
profile, so it is also how a key is rotated, and its name is optional — with one
tenant, "nvfleetint auth add" is the whole setup. The API key is never a flag:
it is read from stdin, so it stays out of shell history.

The profile is the object of these commands, so add/remove/use name it
positionally. Commands that call the API instead select a profile with
--profile; without it they use the current profile.`,
	}

	cmd.AddCommand(newAuthAddCmd())
	cmd.AddCommand(newAuthRemoveCmd())
	cmd.AddCommand(newAuthUseCmd())
	cmd.AddCommand(newAuthListCmd())
	cmd.AddCommand(newAuthStatusCmd())
	// Without this an unrecognized subcommand — `auth update`, which used to
	// exist — prints the help text and exits 0, so a key-rotation script would
	// report success having changed nothing.
	cmdutil.RejectUnknownSubcommands(cmd)

	return cmd
}

// requireProfileNameArg validates the positional <name> these commands act on.
// The profile is what add/remove/use operate on, so it is an argument rather
// than a flag; on API-backed commands --profile means something else entirely
// — which credentials to use (cmdutil.RegisterProfileFlag).
//
// remove and use require the name: defaulting "delete a profile" or "switch
// profiles" to whichever one happens to be called "default" would act on
// something the user never named. Only add, which is creating the thing, can
// safely supply the name itself.
func requireProfileNameArg() cobra.PositionalArgs {
	return cmdutil.RequireSingleArg("profile name")
}

func newAuthAddCmd() *cobra.Command {
	var skipConfirm bool

	cmd := &cobra.Command{
		Use:   "add [<name>]",
		Short: "Add a credential profile, or change an existing one",
		Long: `Store an NGC API key and API URL under a profile name.

The key and the URL are read from stdin, not from flags: a key passed on the
command line is recorded in shell history and visible in the process list for as
long as the command runs.

The name is optional: omitting it targets the profile named "` + config.DefaultProfileName + `",
so a single-tenant setup never has to invent one.

At a terminal the key is typed without being echoed, and the API URL is offered
with a value to accept — the production endpoint for a new profile, the stored
one for an existing profile — so pressing Enter keeps it. An existing profile is
changed in place, which makes this the key-rotation path; pressing Enter at the
key prompt keeps the stored key without ever displaying it.

With stdin piped in, the first line is the API key and the second is the API
URL. Either may be blank to keep the stored or default value. Replacing a stored
key that way needs --yes, since there is nobody to warn.`,
		Example: `  nvfleetint auth add
  nvfleetint auth add prod
  printf '%s\n' "$NGC_API_KEY" | nvfleetint auth add prod --yes
  printf '%s\n%s\n' "$NGC_API_KEY" https://dev.example.com | nvfleetint auth add dev`,
		Args: cmdutil.OptionalSingleArg("profile name"),
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

			// Read once outside the lock, so the questions can offer to keep
			// what is already stored without holding the config lock while
			// waiting for an answer. The write below re-reads under the lock,
			// so this snapshot only shapes the prompts.
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			current, lookupErr := cfg.Profile(name)
			if lookupErr != nil && !errors.Is(lookupErr, config.ErrProfileNotFound) {
				return lookupErr
			}
			exists := lookupErr == nil

			prompt := cmdutil.NewCredentialPrompt(cmd.InOrStdin(), cmd.ErrOrStderr())
			inputs, err := promptForCredentials(prompt, name, current, exists)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			// Keeping both values is a legitimate answer — the whole point of
			// offering them — so it is reported rather than rewritten.
			if exists && !inputs.apiKeySet && !inputs.apiURLSet {
				fmt.Fprintf(out, "Profile %q unchanged.\n", name)
				return nil
			}
			// Replacing a stored key destroys it. At a terminal the key prompt
			// says so and typing a new key is the answer to it; with input
			// piped in nobody saw the warning, so --yes has to carry the intent.
			if destroysStoredKey(current, inputs) && !skipConfirm && !prompt.Interactive() {
				return fmt.Errorf(
					"profile %q already has an API key; re-run with --yes to replace it", name)
			}
			// Only a warning the user actually saw counts as consent, so this
			// stays false when there was no stored key to warn about.
			confirmed := skipConfirm || (prompt.Interactive() && destroysStoredKey(current, inputs))

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

	cmd.Flags().BoolVar(&skipConfirm, "yes", false,
		"Replace a stored API key when stdin is piped in rather than typed")

	return cmd
}

// authAddInputs is what `auth add` was answered with. The *Set fields record
// whether an answer changed anything, which is what separates "leave this
// alone" from "set it to this".
type authAddInputs struct {
	apiKeySet bool
	apiURLSet bool
	apiKey    string
	apiURL    string
}

// promptForCredentials asks for the values `auth add` used to take as flags.
// current is the profile as stored, and is the zero value unless exists.
func promptForCredentials(prompt *cmdutil.CredentialPrompt, name string, current config.Profile, exists bool) (authAddInputs, error) {
	storedKey := strings.TrimSpace(current.APIKey) != ""
	switch {
	case exists && storedKey:
		// Said before the prompt rather than after the fact: at a terminal,
		// typing a new key here is the answer to this warning, and there is no
		// second question to catch a change of mind.
		prompt.Note("Profile %q already exists. Entering a new API key replaces the stored one, which cannot be recovered.", name)
	case exists:
		prompt.Note("Profile %q already exists but has no API key stored.", name)
	}

	apiKey, apiKeySet, err := prompt.APIKey(storedKey)
	if err != nil {
		return authAddInputs{}, err
	}

	// A new profile starts from the production endpoint; an existing one from
	// whatever it already points at, so keeping it is the default answer.
	currentURL := config.DefaultAPIURL
	if exists && strings.TrimSpace(current.APIURL) != "" {
		currentURL = current.APIURL
	}
	apiURL, err := prompt.APIURL(currentURL)
	if err != nil {
		return authAddInputs{}, err
	}

	return authAddInputs{
		apiKeySet: apiKeySet,
		apiKey:    apiKey,
		// An answer that matches what is stored changes nothing, and a new
		// profile has nothing to match, so it always takes the answer.
		apiURLSet: !exists || apiURL != strings.TrimSpace(current.APIURL),
		apiURL:    apiURL,
	}, nil
}

// resolveAuthAddProfile decides what `auth add` writes for one snapshot of the
// config, and reports whether the profile already existed. It is planned
// against the config as it is under the lock, which the answers the user gave
// beforehand may no longer describe.
func resolveAuthAddProfile(cfg config.Config, name string, in authAddInputs) (config.Profile, bool, error) {
	profile, lookupErr := cfg.Profile(name)
	existed := lookupErr == nil

	// The prompt already refuses to leave a keyless profile, so this catches
	// only a profile that vanished between the questions and the write.
	if !existed && !in.apiKeySet {
		return config.Profile{}, false, errors.New("API key is required")
	}

	if in.apiKeySet {
		profile.APIKey = in.apiKey
	}
	// On an existing profile an unchanged answer keeps the stored endpoint; on
	// a new one it takes the default.
	if in.apiURLSet || !existed {
		profile.APIURL = in.apiURL
	}

	return profile, existed, nil
}

// destroysStoredKey reports whether `auth add` is about to replace a service
// key that cannot be recovered — the only thing this command destroys, and so
// the only thing worth warning about. Creating a profile, changing an API URL,
// or supplying the first key for a profile that has none all take nothing away,
// and a warning that fires when nothing is at stake is one people learn to
// ignore. It also has to stay this narrow because the CLI's own remediation
// hints (cmdutil.MissingAPIKeyError, cmdutil.FixAPIURLHint) name an existing profile: were
// those to need --yes, the printed fix would be unusable in CI.
func destroysStoredKey(current config.Profile, in authAddInputs) bool {
	return in.apiKeySet && strings.TrimSpace(current.APIKey) != ""
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
				if err := cmdutil.Confirm(cmd.InOrStdin(), cmd.ErrOrStderr(), summary); err != nil {
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
				fmt.Fprintln(out, "No profiles remain; run `nvfleetint auth add <name>`.")
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
						"%w; run `nvfleetint auth add %s` first",
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
	common := cmdutil.NewCommon()

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List credential profiles",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			commonFlags := cmdutil.ResolveCommon(cmd, common)
			if err := cmdutil.ValidateReadFlags(commonFlags); err != nil {
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
			if commonFlags.Output == clioutput.FormatJSON {
				return clioutput.WriteJSON(out, listOutput)
			}
			if len(listOutput.Profiles) == 0 {
				fmt.Fprintln(out, "No profiles configured. Run `nvfleetint auth add <name>`.")
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
	cmdutil.RegisterOutputFlag(cmd, common)

	return cmd
}

func authListEffectiveProfile(cfg config.Config) (string, string) {
	resolved, err := cfg.Resolve("")
	if err != nil {
		// Only an explicit --profile still fails; a dangling current_profile
		// resolves and comes back as MissingCurrentProfile below.
		return "", ""
	}

	if resolved.MissingCurrentProfile != "" {
		return resolved.MissingCurrentProfile, fmt.Sprintf(
			"Warning: current profile %q is not configured; run `nvfleetint auth use <name>`.",
			resolved.MissingCurrentProfile,
		)
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
	common := cmdutil.NewCommon()

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show which credentials a command would use, and whether they work",
		Example: `  nvfleetint auth status
  nvfleetint auth status --profile dev`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			commonFlags := cmdutil.ResolveCommon(cmd, common)
			if err := cmdutil.ValidateReadFlags(commonFlags); err != nil {
				return err
			}

			resolved, err := cmdutil.ResolveCredentials(commonFlags.Profile)
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
				Warnings:           cmdutil.CredentialWarnings(resolved),
			}
			// Only reach out to the backend when we have both a key and a URL;
			// otherwise the request can't be authenticated.
			if status.APIKeyConfigured && strings.TrimSpace(resolved.APIURL) != "" {
				status.Connection = checkConnection(cmd.Context(), resolved, commonFlags)
			}
			if commonFlags.Output == clioutput.FormatJSON {
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

	cmdutil.RegisterReadFlags(cmd, common)

	return cmd
}

// checkConnection verifies the resolved credentials against the API's
// /v1/auth/status endpoint and returns a human-readable connection state.
// `auth status` is diagnostic, so transport and auth failures are folded into
// the returned string rather than surfaced as command errors.
func checkConnection(ctx context.Context, resolved config.Resolved, common cmdutil.Resolved) string {
	client, err := cmdutil.FromResolved(resolved, common)
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
