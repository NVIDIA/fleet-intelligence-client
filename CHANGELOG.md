# Changelog

## Unreleased

- Expose `AgentUUID` and `BMCMAC` in node responses, preserving the backend
  canonical `nodeUUID` and existing SDK fields. Node describe output includes
  the agent UUID and BMC MAC when available.
