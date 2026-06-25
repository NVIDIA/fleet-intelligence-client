# Automation Usage

`nvfleetctl` supports non-interactive use from scripts, CI jobs, and AI agents.
Prefer JSON output for tool integrations and treat documented JSON fields as
the stable command contract.

## Authentication

Automation can authenticate without writing a config file:

```bash
export NVFLEETCTL_SERVICE_KEY="<ngc-service-key>"
export NVFLEETCTL_API_URL="https://api.fleet-intelligence.nvidia.com"
```

`NVFLEETCTL_SERVICE_KEY` and `NVFLEETCTL_API_URL` override values from
`~/.config/nvfleetctl/config.yaml` for the current process. `NVFLEETCTL_API_URL`
is optional when using the default production API.

Humans can still persist credentials with:

```bash
nvfleetctl auth login --key "<ngc-service-key>"
```

## JSON Output

Use `--output json` or `-o json` for machine-readable output:

```bash
nvfleetctl auth status --output json
nvfleetctl version --output json
nvfleetctl node list --all --output json
nvfleetctl report verify --csv report.csv --bundle report.sig.bundle --output json
```

Single-page API-backed list and describe commands preserve the raw API JSON
shape. All-page list commands normalize paginated responses into:

```json
{
  "items": [],
  "pagination": {
    "page": 0,
    "pageSize": 100,
    "total": 0,
    "hasMore": false,
    "pagesFetched": 1
  }
}
```

Commands that report local status use compact status objects. For example:

```json
{"status":"verified"}
```

## Errors

When a command is run through the installed `nvfleetctl` binary with
`--output json`, failures are written to stderr as a single JSON object and the
process exits nonzero:

```json
{
  "error": {
    "code": "command_error",
    "message": "--csv is required"
  }
}
```

API failures use `code: "api_error"` and may include `statusCode`, `status`,
and `details`.
