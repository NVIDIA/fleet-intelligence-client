# CLI guide

Use `nvfleetint` to inspect fleet health, inventory, alerts, events, and
reports. Run `nvfleetint <command> --help` for every available flag.

## Authentication

Create an NGC [personal API key](https://docs.nvidia.com/ngc/latest/ngc-user-guide.html#generating-a-personal-api-key)
or [service key](https://org.ngc.nvidia.com/identity-access/service-keys), then
store it locally:

```bash
nvfleetint auth login --key <your-ngc-api-key>
nvfleetint auth status
```

On Linux and macOS, credentials are stored in
`~/.config/nvfleetint/config.yaml` with file mode `0600`. On Windows, they are
stored in `%USERPROFILE%\.config\nvfleetint\config.yaml`; access follows Windows
ACLs, and the Go file mode controls only whether the file is writable. To use a
different endpoint, pass `--api-url` during login.

## Common commands

```bash
# Fleet summary and inventory
nvfleetint overview
nvfleetint computezone list
nvfleetint nodegroup list
nvfleetint node list
nvfleetint node describe <node-uuid>

# Health, alerts, and events
nvfleetint node health <node-uuid> \
  --start 2026-05-01T00:00:00Z --end 2026-05-08T00:00:00Z
nvfleetint alert list --severity Critical
nvfleetint alert timeline --node <node-uuid>
nvfleetint event list --window 24h
nvfleetint event buckets --window 24h

# Tags and reports
nvfleetint tag list --prefix gpu
nvfleetint report inventory
nvfleetint report error --window 24h
```

List commands support shared flags including `--all`, `--page`, `--page-size`,
`--timeout`, and `--output json`.

## CSV reports

Write an inventory report to a file:

```bash
nvfleetint report inventory --format csv > inventory.csv
```

Download and verify a signed inventory bundle:

```bash
nvfleetint report inventory --format csv --signed \
  --output-path inventory-report.zip
unzip -l inventory-report.zip
unzip inventory-report.zip

# Use the extracted names shown by unzip -l.
nvfleetint report verify \
  --csv <extracted-csv-path> \
  --bundle <extracted-sig-bundle-path>
```

Use `--key signing-key.pub` with `report verify` to supply a local public key.

## Automation

For scripts and CI jobs, authenticate without writing a configuration file:

```bash
export NVFLEETINT_SERVICE_KEY="<ngc-service-key>"
export NVFLEETINT_API_URL="https://api.fleet-intelligence.nvidia.com"
```

`NVFLEETINT_API_URL` is optional when using the production API. Environment
variables override the saved configuration for the current process.

Use `--output json` or `-o json` for machine-readable output. API-backed
commands preserve the API response shape for a single page. With `--all`, list
commands return a normalized object:

```json
{
  "items": [],
  "pagination": {
    "page": 1,
    "pageSize": 100,
    "total": 0,
    "hasMore": false,
    "pagesFetched": 1
  }
}
```

Successful JSON is written to stdout. When JSON output is active, failures are
written to stderr in this form:

```json
{
  "error": {
    "code": "command_error",
    "message": "--csv is required"
  }
}
```

Command validation and local failures use `error.code: "command_error"`. API
failures use `error.code: "api_error"` and may also include `statusCode`,
`status`, and `details`.

Exit code `0` means success, `1` means a general failure, and `77` means a 401
or 403 API error reached command failure handling. `auth status` is diagnostic:
it reports those responses as `connection: "unauthorized"` and exits `0`.
Prefer `error.code` over the exit code when handling JSON errors.

Commands that stream CSV do not accept `--output`. `auth login` and
`auth logout` also do not provide JSON output.
