#!/usr/bin/env python3
"""Import the four page/count operations and missing schemas from a backend spec.

Existing operations and model definitions are preserved to avoid unrelated SDK
contract changes. Pass the reviewed backend OpenAPI JSON as the only argument.
"""
import copy
import json
import sys
from pathlib import Path

import yaml

source = json.loads(Path(sys.argv[1]).read_text())
target = Path(__file__).resolve().parents[1] / "api/openapi/openapi.yaml"
text = target.read_text()
current = yaml.safe_load(text)
paths = ["/v1/events/page", "/v1/events/count", "/v1/xid/bursts/page", "/v1/xid/bursts/count"]
for path in paths:
    if path in current["paths"] and current["paths"][path] != source["paths"][path]:
        raise SystemExit(f"Existing operation differs from reviewed source: {path}; review the contract change explicitly")
operations = {path: copy.deepcopy(source["paths"][path]) for path in paths if path not in current["paths"]}
schemas = {}


def collect(value):
    if isinstance(value, dict):
        ref = value.get("$ref", "")
        if ref.startswith("#/components/schemas/"):
            name = ref.removeprefix("#/components/schemas/")
            if name in current["components"]["schemas"] and current["components"]["schemas"][name] != source["components"]["schemas"][name]:
                raise SystemExit(f"Existing schema differs from reviewed source: {name}; review the model change explicitly")
            if name not in current["components"]["schemas"] and name not in schemas:
                schemas[name] = copy.deepcopy(source["components"]["schemas"][name])
                collect(schemas[name])
        for child in value.values():
            collect(child)
    elif isinstance(value, list):
        for child in value:
            collect(child)


collect(operations)
if operations:
    fragment = yaml.safe_dump({"paths": operations}, sort_keys=False, allow_unicode=True).removeprefix("paths:\n")
    text = text.replace("paths:\n", "paths:\n" + fragment, 1)
if schemas:
    fragment = yaml.safe_dump({"components": {"schemas": schemas}}, sort_keys=False, allow_unicode=True).removeprefix("components:\n  schemas:\n")
    text = text.replace("  schemas:\n", "  schemas:\n" + fragment, 1)
target.write_text(text)
print(f"Imported {len(operations)} operations and {len(schemas)} response schemas")
