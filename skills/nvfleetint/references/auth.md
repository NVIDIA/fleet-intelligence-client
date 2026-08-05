# Installation and profiles

Read only for setup, key rotation, or credential-source diagnosis.

## Installation

Exit 127/`command not found` means the CLI is absent. Don't build from source;
use the matching release asset from
<https://github.com/NVIDIA/fleet-intelligence-client/releases>, put it on
`PATH`, then run `nvfleetint auth status`.

## Profiles

Profiles pair API URL and key in `~/.config/nvfleetint/config.yaml` (0600):

```bash
nvfleetint auth list
nvfleetint auth status [--profile <name>]
nvfleetint auth add --api-key <key>                    # default profile
nvfleetint auth add <name> --api-key <key> [--api-url https://...]
nvfleetint auth use <name>
nvfleetint auth add <name> --api-key <new-key> --yes   # rotate
nvfleetint auth remove <name>
```

`auth add/remove/use` take a positional name, not `--profile`; add's name is
optional and defaults to `default`. There is no `auth update`: add updates an
existing profile. Run `auth list` before add so a typo cannot overwrite another
profile. Use `--yes` only when the user explicitly requested key replacement.

Require HTTPS except localhost. Never request, print, or log a key.

## Diagnose effective credentials

`auth status` reports Profile, API URL, API-key source, and Connection but exits
0 on bad auth. Require `Connection: ok`.

Without explicit `--profile`, the current profile is overlaid by
`NVFLEETINT_API_KEY`/`NVFLEETINT_API_URL`; explicit profile ignores both. If
status shows an unexpected environment source, ask the user to unset the
variables without printing their values.

Exit 77 or HTTP 401/403 is auth/permission failure. Direct the user to create an
NGC service key at
<https://org.ngc.nvidia.com/identity-access/service-keys> and run auth add.
Never interpret auth failure as an empty fleet.
