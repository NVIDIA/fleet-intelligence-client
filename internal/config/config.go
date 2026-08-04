// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// Package config stores and resolves nvfleetint credential profiles.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

const (
	// DefaultAPIURL is the production Fleet Intelligence API root.
	DefaultAPIURL = "https://api.fleet-intelligence.nvidia.com"
	// DefaultProfileName is the profile `auth add` targets when no name is
	// given, so single-tenant setup never has to invent one.
	DefaultProfileName = "default"
	// EnvAPIURL overrides the resolved API URL for the current process.
	EnvAPIURL = "NVFLEETINT_API_URL"
	// EnvAPIKey overrides the resolved API key for the current process.
	EnvAPIKey = "NVFLEETINT_API_KEY"
	// EnvLegacyAPIKey is the pre-rename name of EnvAPIKey. It is deliberately
	// not read — it exists so a command that finds no credentials can point at
	// the variable the user actually exported instead of failing with an error
	// that never mentions it.
	EnvLegacyAPIKey = "NVFLEETINT_SERVICE_KEY"
	// EnvProfile selects a stored profile for the current process.
	EnvProfile = "NVFLEETINT_PROFILE"

	dirName  = "nvfleetint"
	fileName = "config.yaml"
	fileMode = 0o600
	dirMode  = 0o700
	lockMode = 0o600

	lockPollInterval = 25 * time.Millisecond
	lockStaleAfter   = 5 * time.Minute
	lockTimeout      = 30 * time.Second

	maxProfileNameLength = 64
)

// Errors callers can match on to add command-specific hints.
var (
	// ErrProfileNotFound reports a reference to a profile that is not stored.
	ErrProfileNotFound = errors.New("profile not found")
	// ErrProfileExists reports an attempt to add a profile that already exists.
	ErrProfileExists = errors.New("profile already exists")
	// ErrNoProfile reports that no profile is selected and no credentials are available.
	ErrNoProfile = errors.New("no profile configured")
)

// Profile names are used as YAML mapping keys and appear in shell commands, so
// keep them to characters that need no quoting anywhere, and require a leading
// alphanumeric so a name is never mistaken for a flag or a hidden file.
var profileNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ReservedProfileName is the token printed when no profile is in use, so it
// cannot also name one.
const ReservedProfileName = "none"

// Profile is one named set of Fleet Intelligence credentials.
type Profile struct {
	APIURL string `yaml:"api_url"`
	APIKey string `yaml:"api_key"`
}

// Config is the on-disk configuration file.
type Config struct {
	CurrentProfile string             `yaml:"current_profile,omitempty"`
	Profiles       map[string]Profile `yaml:"profiles,omitempty"`
}

// Source names where a resolved value came from.
type Source string

const (
	// SourceProfile marks a value read from a stored profile.
	SourceProfile Source = "profile"
	// SourceEnvironment marks a value read from an environment variable.
	SourceEnvironment Source = "environment"
	// SourceDefault marks a value that fell back to a built-in default.
	SourceDefault Source = "default"
)

// Resolved is the credential set a command should use, plus where each half of
// it came from so `auth status` can explain itself.
type Resolved struct {
	Profile            string
	APIURL             string
	APIKey             string
	APIURLSource       Source
	APIKeySource       Source
	ProfilesConfigured bool
	// EnvIgnored names the credential environment variables that were set but
	// skipped because a profile was selected explicitly. Only the variables
	// actually set are listed, so a note built from it cannot claim more than
	// what is really in the environment.
	EnvIgnored []string
	// MissingCurrentProfile names the profile current_profile points at when
	// that profile is no longer stored. Resolution continues instead of
	// failing, so callers must treat this as the reason a resolved set has no
	// profile in it rather than as an aside.
	MissingCurrentProfile string
	// ConfigError reports a configuration file that could not be read or
	// parsed while the environment still supplied a complete credential set.
	// Resolution succeeded, so this is a warning to surface, not a failure —
	// without it the stored profiles simply appear not to exist.
	ConfigError error
}

// Path returns the location of the configuration file.
func Path() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(homeDir) == "" {
		return "", errors.New("home directory is required")
	}

	return filepath.Join(homeDir, ".config", dirName, fileName), nil
}

// Load reads the configuration file, returning an empty config when it is absent.
func Load() (Config, error) {
	path, err := Path()
	if err != nil {
		return Config{}, err
	}

	return loadFromPath(path)
}

func loadFromPath(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, nil
		}
		return Config{}, fmt.Errorf("read config %q: %w", path, err)
	}

	var cfg Config
	// Unknown keys are ignored, so a pre-profile flat config decodes to zero
	// profiles rather than failing. There is deliberately no migration.
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("read config %q: %w", path, err)
	}

	return cfg.normalized(false), nil
}

// Save writes the configuration file with owner-only permissions.
func Save(cfg Config) error {
	path, err := Path()
	if err != nil {
		return err
	}

	return withConfigLock(path, func() error {
		return saveToPath(path, cfg)
	})
}

func saveToPath(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return err
	}

	cfg = cfg.normalized(true)
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	return writeFileAtomic(path, data)
}

// Edit locks the config, loads the latest contents, applies mutate, and writes
// the complete file back. It is used by auth mutators so concurrent commands
// cannot overwrite each other's profile changes.
func Edit(mutate func(*Config) error) (Config, error) {
	path, err := Path()
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	err = withConfigLock(path, func() error {
		var loadErr error
		cfg, loadErr = loadFromPath(path)
		if loadErr != nil {
			return loadErr
		}
		if err := mutate(&cfg); err != nil {
			return err
		}
		if err := saveToPath(path, cfg); err != nil {
			return err
		}
		cfg = cfg.normalized(true)
		return nil
	})
	if err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// writeFileAtomic writes to a temporary file in the same directory and renames
// it over the target. The file holds every stored API key, so a torn write
// would destroy credentials rather than just lose the newest change.
func writeFileAtomic(path string, data []byte) error {
	writePath, err := resolveWritePath(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(writePath), dirMode); err != nil {
		return err
	}

	file, err := os.CreateTemp(filepath.Dir(writePath), ".config-*.yaml")
	if err != nil {
		return err
	}
	tempPath := file.Name()
	// A no-op once the rename below succeeds; on any earlier failure this is
	// what keeps a half-written file with credentials in it off the disk.
	defer func() { _ = os.Remove(tempPath) }()

	if err := writeAndClose(file, data); err != nil {
		return err
	}

	return os.Rename(tempPath, writePath)
}

func writeAndClose(file *os.File, data []byte) error {
	defer func() { _ = file.Close() }()

	// os.CreateTemp already creates the file 0600; set it explicitly so the
	// permissions come from one named constant.
	if err := file.Chmod(fileMode); err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}

	return file.Close()
}

func resolveWritePath(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return path, nil
		}
		return "", err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return path, nil
	}

	target, err := os.Readlink(path)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(path), target)
	}

	return filepath.Clean(target), nil
}

func withConfigLock(path string, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return err
	}

	lockPath := path + ".lock"
	lockFile, err := acquireLock(lockPath)
	if err != nil {
		return err
	}
	defer func() {
		_ = lockFile.Close()
		_ = os.Remove(lockPath)
	}()

	return fn()
}

func acquireLock(lockPath string) (*os.File, error) {
	deadline := time.Now().Add(lockTimeout)
	for {
		file, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE|os.O_EXCL, lockMode)
		if err == nil {
			_, _ = fmt.Fprintf(file, "%d\n", os.Getpid())
			return file, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}

		info, statErr := os.Stat(lockPath)
		if statErr == nil && time.Since(info.ModTime()) > lockStaleAfter {
			// If the stale lock can't be removed, fall through to the deadline
			// check and sleep rather than spinning on the same failure.
			if removeErr := os.Remove(lockPath); removeErr == nil {
				continue
			}
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("config is locked by another nvfleetint process: %s", lockPath)
		}
		time.Sleep(lockPollInterval)
	}
}

// Resolve loads the config and resolves the credentials for profileName.
func Resolve(profileName string) (Resolved, error) {
	cfg, err := Load()
	if err != nil {
		if !canResolveFromEnvironmentOnly(profileName) {
			return Resolved{}, err
		}
		// The environment holds a complete credential set, so an unreadable
		// config must not stop the command. It is still reported: a corrupt
		// file otherwise looks exactly like an absent one, and `auth status`
		// would claim no profiles are configured while `auth list` fails.
		resolved, resolveErr := Config{}.Resolve("")
		if resolveErr != nil {
			return Resolved{}, resolveErr
		}
		resolved.ConfigError = err

		return resolved, nil
	}

	return cfg.Resolve(profileName)
}

func canResolveFromEnvironmentOnly(profileName string) bool {
	return strings.TrimSpace(profileName) == "" &&
		strings.TrimSpace(os.Getenv(EnvProfile)) == "" &&
		strings.TrimSpace(os.Getenv(EnvAPIKey)) != ""
}

// Resolve returns the credentials a command should use.
//
// Precedence, highest first:
//
//  1. an explicitly named profile (--profile), whose values are used verbatim;
//  2. NVFLEETINT_PROFILE, treated the same way;
//  3. the current profile, with NVFLEETINT_API_KEY / NVFLEETINT_API_URL
//     overlaid field by field.
//
// An explicit selection deliberately ignores the credential environment
// variables: with several tenants configured, a stale NVFLEETINT_API_KEY
// would otherwise send one tenant's key to another tenant's endpoint.
//
// A current profile that is no longer stored is reported in
// Resolved.MissingCurrentProfile rather than returned as an error, so the
// environment overlay still applies. Only an explicit selection — the two
// higher-precedence cases, where the user named the profile — fails outright.
func (c Config) Resolve(profileName string) (Resolved, error) {
	profilesConfigured := len(c.Profiles) > 0
	explicit := strings.TrimSpace(profileName)
	explicitFromEnv := false
	if explicit == "" {
		explicit = strings.TrimSpace(os.Getenv(EnvProfile))
		explicitFromEnv = explicit != ""
	}

	envAPIURL := strings.TrimSpace(os.Getenv(EnvAPIURL))
	envAPIKey := strings.TrimSpace(os.Getenv(EnvAPIKey))

	if explicit != "" {
		profile, apiURLSource, err := c.profileForResolve(explicit)
		if err != nil {
			if explicitFromEnv {
				return Resolved{}, fmt.Errorf("%w from %s", err, EnvProfile)
			}
			return Resolved{}, err
		}
		var ignored []string
		if envAPIKey != "" {
			ignored = append(ignored, EnvAPIKey)
		}
		if envAPIURL != "" {
			ignored = append(ignored, EnvAPIURL)
		}
		return Resolved{
			Profile:            explicit,
			APIURL:             profile.APIURL,
			APIKey:             profile.APIKey,
			APIURLSource:       apiURLSource,
			APIKeySource:       SourceProfile,
			ProfilesConfigured: profilesConfigured,
			EnvIgnored:         ignored,
		}, nil
	}

	resolved := Resolved{
		APIURL:             DefaultAPIURL,
		APIURLSource:       SourceDefault,
		APIKeySource:       SourceDefault,
		ProfilesConfigured: profilesConfigured,
	}
	if current := strings.TrimSpace(c.CurrentProfile); current != "" {
		profile, apiURLSource, err := c.profileForResolve(current)
		switch {
		case errors.Is(err, ErrProfileNotFound):
			// Nobody asked for this profile by name — it is a selection left
			// behind in the file. Failing here would take down every command
			// including `auth status`, the one that explains what happened,
			// even when the environment below carries working credentials.
			resolved.MissingCurrentProfile = current
		case err != nil:
			return Resolved{}, fmt.Errorf("%w from current_profile", err)
		default:
			resolved.Profile = current
			resolved.APIURL = profile.APIURL
			resolved.APIKey = profile.APIKey
			resolved.APIURLSource = apiURLSource
			resolved.APIKeySource = SourceProfile
		}
	}

	if envAPIURL != "" {
		resolved.APIURL = envAPIURL
		resolved.APIURLSource = SourceEnvironment
	}
	if envAPIKey != "" {
		resolved.APIKey = envAPIKey
		resolved.APIKeySource = SourceEnvironment
	}

	return resolved, nil
}

// Profile returns the named profile.
func (c Config) Profile(name string) (Profile, error) {
	name = strings.TrimSpace(name)
	profile, ok := c.Profiles[name]
	if !ok {
		return Profile{}, fmt.Errorf("%w: %q", ErrProfileNotFound, name)
	}
	profile = profile.normalized(true)

	return profile, nil
}

func (c Config) profileForResolve(name string) (Profile, Source, error) {
	name = strings.TrimSpace(name)
	profile, ok := c.Profiles[name]
	if !ok {
		return Profile{}, "", fmt.Errorf("%w: %q", ErrProfileNotFound, name)
	}

	profile = profile.normalized(false)
	apiURLSource := SourceProfile
	if profile.APIURL == "" {
		profile.APIURL = DefaultAPIURL
		apiURLSource = SourceDefault
	}

	return profile, apiURLSource, nil
}

// ProfileNames returns the stored profile names in sorted order.
func (c Config) ProfileNames() []string {
	names := make([]string, 0, len(c.Profiles))
	for name := range c.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)

	return names
}

// AddProfile stores a new profile, refusing to overwrite an existing one. The
// first profile added also becomes the current profile.
func (c *Config) AddProfile(name string, profile Profile) error {
	name = strings.TrimSpace(name)
	if err := ValidateProfileName(name); err != nil {
		return err
	}
	if _, ok := c.Profiles[name]; ok {
		return fmt.Errorf("%w: %q", ErrProfileExists, name)
	}

	c.setProfile(name, profile)
	if strings.TrimSpace(c.CurrentProfile) == "" || !c.hasCurrentProfile() {
		c.CurrentProfile = name
	}

	return nil
}

// UpdateProfile replaces the stored values for an existing profile.
func (c *Config) UpdateProfile(name string, profile Profile) error {
	name = strings.TrimSpace(name)
	if err := ValidateProfileName(name); err != nil {
		return err
	}
	if _, ok := c.Profiles[name]; !ok {
		return fmt.Errorf("%w: %q", ErrProfileNotFound, name)
	}

	c.setProfile(name, profile)
	if strings.TrimSpace(c.CurrentProfile) != "" && !c.hasCurrentProfile() {
		c.CurrentProfile = name
	}

	return nil
}

// RemoveProfile deletes a profile. Removing the current profile always clears
// the selection — no profile is auto-selected in its place, so the next one
// must be chosen explicitly with `auth use`.
func (c *Config) RemoveProfile(name string) error {
	name = strings.TrimSpace(name)
	if _, ok := c.Profiles[name]; !ok {
		return fmt.Errorf("%w: %q", ErrProfileNotFound, name)
	}

	delete(c.Profiles, name)
	if c.CurrentProfile == name || !c.hasCurrentProfile() {
		c.CurrentProfile = ""
	}

	return nil
}

// UseProfile selects an existing profile as the default.
func (c *Config) UseProfile(name string) error {
	name = strings.TrimSpace(name)
	if _, ok := c.Profiles[name]; !ok {
		return fmt.Errorf("%w: %q", ErrProfileNotFound, name)
	}
	c.CurrentProfile = name

	return nil
}

// ValidateProfileName rejects names that would need quoting in YAML or a shell.
func ValidateProfileName(name string) error {
	name = strings.TrimSpace(name)
	if strings.TrimSpace(name) == "" {
		return errors.New("profile name is required")
	}
	if len(name) > maxProfileNameLength {
		return fmt.Errorf("profile name must be at most %d characters", maxProfileNameLength)
	}
	if !profileNamePattern.MatchString(name) {
		return fmt.Errorf(
			"invalid profile name %q: use letters, digits, '.', '_' or '-', starting with a letter or digit",
			name,
		)
	}
	if strings.EqualFold(name, ReservedProfileName) {
		return fmt.Errorf("profile name %q is reserved", name)
	}

	return nil
}

func (c *Config) setProfile(name string, profile Profile) {
	profile = profile.normalized(true)
	if c.Profiles == nil {
		c.Profiles = make(map[string]Profile)
	}
	c.Profiles[name] = profile
}

func (c Config) hasCurrentProfile() bool {
	current := strings.TrimSpace(c.CurrentProfile)
	if current == "" {
		return false
	}
	_, ok := c.Profiles[current]
	return ok
}

func (c Config) normalized(defaultAPIURL bool) Config {
	normalized := Config{
		CurrentProfile: strings.TrimSpace(c.CurrentProfile),
	}
	if len(c.Profiles) > 0 {
		normalized.Profiles = make(map[string]Profile, len(c.Profiles))
	}
	for name, profile := range c.Profiles {
		normalized.Profiles[name] = profile.normalized(defaultAPIURL)
	}

	return normalized
}

func (p Profile) normalized(defaultAPIURL bool) Profile {
	p.APIURL = strings.TrimSpace(p.APIURL)
	p.APIKey = strings.TrimSpace(p.APIKey)
	if defaultAPIURL && p.APIURL == "" {
		p.APIURL = DefaultAPIURL
	}

	return p
}
