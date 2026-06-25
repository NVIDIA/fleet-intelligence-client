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

Use `--output json` or `-o json` for machine-readable output.

### Supported commands

| Command | Notes |
|---------|-------|
| `nvfleetctl auth status --output json` | Returns auth config and connection state |
| `nvfleetctl version --output json` | Returns binary version metadata |
| `nvfleetctl node list --output json` | Single page; use `--all` for all pages |
| `nvfleetctl node describe <id> --output json` | Raw API shape |
| `nvfleetctl alert list --output json` | Single page; use `--all` for all pages |
| `nvfleetctl alert describe <id> --output json` | Raw API shape |
| `nvfleetctl alert timeline --output json` | Single page; use `--all` for all pages |
| `nvfleetctl compute-zone list --output json` | Single page; use `--all` for all pages |
| `nvfleetctl node-group list --output json` | Single page; use `--all` for all pages |
| `nvfleetctl report inventory --output json` | Default `--format json`; returns paginated nodes |
| `nvfleetctl report inventory --format csv --signed --output json` | Returns `{"status":"written","path":"..."}` |
| `nvfleetctl report error --view overview --window 24h --output json` | All views supported |
| `nvfleetctl report verify --csv f.csv --bundle f.sig.bundle --output json` | Returns `{"status":"verified"}` |

### Not supported

- `report inventory --format csv` (unsigned) — streams raw CSV bytes; `--output` is rejected
- `report error --format csv` — streams raw CSV bytes; `--output` is rejected
- `auth login`, `auth logout` — interactive; no JSON output

### Output shapes

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

Commands that report local status use compact status objects, for example:

```json
{"status":"verified"}
```

## Errors

When `--output json` is active, failures are written to **stderr** as a JSON
object and the process exits nonzero. Success output goes to **stdout**. Capture
both streams to handle all outcomes.

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

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | General error (API error, file I/O, unexpected failure) |
| `77` | Auth/permission failure (HTTP 401 or 403 from the API) |

Agents should prefer parsing the JSON `error.code` field over relying solely on
the exit code, as it carries more detail.
