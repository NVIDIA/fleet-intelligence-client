# Examples

Common `nvfleetctl` workflows. Run `nvfleetctl <command> --help` for the full
flag list.

## Authentication

Generate an NGC service key at
<https://org.ngc.nvidia.com/identity-access/service-keys>. For detailed
instructions, see the
[Fleet Intelligence API reference](https://docs.nvidia.com/fleet-intelligence/latest/api-reference.html).

```bash
# Store a service key with the default API URL
nvfleetctl auth login --key <your-ngc-service-key>

# Use a custom API URL
nvfleetctl auth login --key <your-ngc-service-key> \
  --api-url https://api.fleet-intelligence.nvidia.com

# Inspect or remove local credentials
nvfleetctl auth status
nvfleetctl auth logout
```

Credentials are stored in `~/.config/nvfleetctl/config.yaml` with file mode
`0600`.

The API URL must be `https`. Every request carries the service key in an
`Authorization` header, so plain `http` is rejected — except for loopback hosts
(`127.0.0.1`, `::1`, `localhost`), which stay available for local development.
The same rule applies to `NVFLEETCTL_API_URL` and to a hand-edited config file.

## Fleet Inventory

```bash
# Compute zones
nvfleetctl computezone list
nvfleetctl computezone list --view basic

# Node groups
nvfleetctl nodegroup list
nvfleetctl nodegroup list --health Degraded,Unhealthy
nvfleetctl nodegroup list --gpu-type H100 --sort-by health --order desc

# Nodes
nvfleetctl node list
nvfleetctl node list --health Degraded,Unhealthy --hostname gpu-node
nvfleetctl node list --agent-status Offline --sort-by hostname --order asc
nvfleetctl node describe <node-uuid>
nvfleetctl node describe <node-uuid> --output json
```

List commands support shared flags such as `--all`, `--page`, `--page-size`,
`--timeout`, and `--output json`.

## Alerts

```bash
nvfleetctl alert list
nvfleetctl alert list --severity Critical --node <node-uuid>
nvfleetctl alert timeline
nvfleetctl alert timeline --active
nvfleetctl alert timeline --node <node-uuid>
nvfleetctl alert describe <alert-uuid> --node <node-uuid>
```

## Reports

```bash
# Inventory
nvfleetctl report inventory
nvfleetctl report inventory --all --output json
nvfleetctl report inventory --format csv > inventory.csv

# Error reports
nvfleetctl report error --window 24h
nvfleetctl report error --view list --group-by error --window 168h
nvfleetctl report error --view list --group-by node \
  --start 2026-05-01T00:00:00Z --end 2026-05-08T00:00:00Z
nvfleetctl report error --view graph --window 24h
```

## Signed Inventory Verification

Download a signed inventory CSV bundle:

```bash
nvfleetctl report inventory --format csv --signed
unzip inventory-report.zip
```

Verify with the signing key fetched from the configured API:

```bash
nvfleetctl report verify \
  --csv inventory_report_<timestamp>.csv \
  --bundle inventory_report_<timestamp>.sig.bundle
```

Verify offline with a local public key:

```bash
nvfleetctl report verify \
  --csv inventory_report_<timestamp>.csv \
  --bundle inventory_report_<timestamp>.sig.bundle \
  --key signing-key.pub
```

On success, the command prints `Verified OK`.
