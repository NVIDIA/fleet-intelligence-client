# Example investigation

> User: "Node gpu-h100-3271 has been flaky — write me an RCA."

1. Confirm authentication with `nvfleetint auth status`.
2. Resolve the hostname with
   `nvfleetint node list --hostname gpu-h100-3271 --view basic --all --output json`
   and confirm the single matching UUID.
3. Pin `<start>` and `<end>` once, then collect Batch B together: `node
   describe`, `node health --start <start> --end <end>`, `event list --node
   <node_uuid> --start <start> --end <end> --all`, `event buckets --node
   <node_uuid> --start <start> --end <end>`, and `alert timeline --node
   <node_uuid>` in full and with `--active`.
4. Validate completeness, then apply the fast-path gate:
   - For a trivial incident with no more than one active alert, zero events, and
     one health segment, describe the single active alert and write the report.
   - For multiple active alerts, bursts, or flapping, select the three to five
     most relevant alert UUIDs and invoke `alert describe` separately for each
     UUID. The independent invocations may run in parallel.
5. Analyze the timeline, root cause and confidence, and RCCA actions.
6. Produce `node-rca-rcca-gpu-h100-3271.html`, clean the validated scratch
   directory, and return the report path with node, collection time, and window.
