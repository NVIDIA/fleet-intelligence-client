# Example investigation

> User: "Node gpu-h100-3271 has been flaky — write me an RCA."

1. Confirm authentication with `nvfleetctl auth status`.
2. Resolve the hostname with
   `nvfleetctl node list --hostname gpu-h100-3271 --view detail --all --output json`
   and confirm the single matching UUID.
3. Set a seven-day window and collect Batch B together: `node describe`, `node
   health --start <T-7d> --end <T>`, `event list --node <node_uuid> --window
   168h --all`, `event buckets --node <node_uuid> --window 168h`, and `alert
   timeline --node <node_uuid>` in full and with `--active`.
4. Validate completeness, then apply the fast-path gate:
   - For a trivial incident with no more than one active alert, zero events, and
     one health segment, describe the single active alert and write the report.
   - For multiple active alerts, bursts, or flapping, describe the three to five
     most relevant alert UUIDs as one batch.
5. Analyze the timeline, root cause and confidence, and RCCA actions.
6. Produce `node-rca-rcca-gpu-h100-3271.html`, clean the validated scratch
   directory, and return the report path with node, collection time, and window.
