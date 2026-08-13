#!/usr/bin/env python3
"""Convert swag OAS2 JSON to docs/openapi.yaml (OpenAPI 3.0.3, HTTP bearer)."""

from __future__ import annotations

import json
import sys
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parents[1]
SRC = ROOT / "docs" / "swagger" / "swagger.json"
DEST = ROOT / "docs" / "openapi.yaml"


def ref_oas3(s: str) -> str:
    return s.replace("#/definitions/", "#/components/schemas/")


def convert_schema(node: Any) -> Any:
    if isinstance(node, list):
        return [convert_schema(x) for x in node]
    if not isinstance(node, dict):
        return node
    out: dict[str, Any] = {}
    for k, v in node.items():
        if k == "$ref" and isinstance(v, str):
            out[k] = ref_oas3(v)
        else:
            out[k] = convert_schema(v)
    return out


def convert_parameters(params: list[dict], consumes: list[str]) -> tuple[list[dict], dict | None]:
    query_params: list[dict] = []
    request_body = None
    content_type = (consumes or ["application/json"])[0]
    for p in params or []:
        if p.get("in") == "body":
            schema = convert_schema(p.get("schema", {}))
            request_body = {
                "description": p.get("description", ""),
                "required": bool(p.get("required")),
                "content": {content_type: {"schema": schema}},
            }
            continue
        np: dict[str, Any] = {
            "name": p["name"],
            "in": p.get("in", "query"),
        }
        if p.get("description"):
            np["description"] = p["description"]
        if p.get("required"):
            np["required"] = True
        schema: dict[str, Any] = {}
        for field in ("type", "format", "enum", "default", "items", "minimum", "maximum"):
            if field in p:
                schema[field] = convert_schema(p[field])
        if schema:
            np["schema"] = schema
        query_params.append(np)
    return query_params, request_body


def convert_responses(responses: dict, produces: list[str]) -> dict:
    content_type = (produces or ["application/json"])[0]
    out: dict[str, Any] = {}
    for code, resp in (responses or {}).items():
        item: dict[str, Any] = {"description": resp.get("description") or ""}
        if "schema" in resp:
            item["content"] = {content_type: {"schema": convert_schema(resp["schema"])}}
        out[str(code)] = item
    return out


def convert_paths(paths: dict) -> dict:
    out: dict[str, Any] = {}
    for path, item in (paths or {}).items():
        op_map: dict[str, Any] = {}
        for method, op in item.items():
            if method.startswith("x-"):
                continue
            consumes = op.get("consumes") or []
            produces = op.get("produces") or []
            params, body = convert_parameters(op.get("parameters") or [], consumes)
            converted: dict[str, Any] = {}
            for key in ("tags", "summary", "description", "operationId", "deprecated", "security"):
                if key in op:
                    converted[key] = op[key]
            if params:
                converted["parameters"] = params
            if body:
                converted["requestBody"] = body
            converted["responses"] = convert_responses(op.get("responses") or {}, produces)
            op_map[method] = converted
        out[path] = op_map
    return out


def convert_security_schemes(defs: dict) -> dict:
    out: dict[str, Any] = {}
    for name, scheme in (defs or {}).items():
        if name == "BearerAuth" or (
            scheme.get("type") == "apiKey" and scheme.get("name") == "Authorization"
        ):
            out[name] = {
                "type": "http",
                "scheme": "bearer",
                "bearerFormat": "JWT",
                "description": scheme.get("description") or "JWT access token",
            }
            continue
        out[name] = dict(scheme)
    return out


def to_oas3(oas2: dict) -> dict:
    host = oas2.get("host") or "localhost:8080"
    base = oas2.get("basePath") or "/"
    schemes = oas2.get("schemes") or ["http"]
    servers = [{"url": f"{sch}://{host}{base}".rstrip("/") or "/"} for sch in schemes]

    components: dict[str, Any] = {}
    if oas2.get("definitions"):
        components["schemas"] = convert_schema(oas2["definitions"])
    schemes_out = convert_security_schemes(oas2.get("securityDefinitions") or {})
    if schemes_out:
        components["securitySchemes"] = schemes_out

    doc: dict[str, Any] = {
        "openapi": "3.0.3",
        "info": oas2.get("info") or {"title": "API", "version": "1.0"},
        "servers": servers,
        "paths": convert_paths(oas2.get("paths") or {}),
    }
    if components:
        doc["components"] = components
    if oas2.get("tags"):
        doc["tags"] = oas2["tags"]
    if oas2.get("security"):
        doc["security"] = oas2["security"]
    return doc


def dump_yaml(value: Any, indent: int = 0) -> str:
    sp = "  " * indent
    if value is None:
        return "null"
    if isinstance(value, bool):
        return "true" if value else "false"
    if isinstance(value, int) and not isinstance(value, bool):
        return str(value)
    if isinstance(value, float):
        return str(value)
    if isinstance(value, str):
        looks_number = False
        try:
            float(value)
            looks_number = True
        except ValueError:
            pass
        if (
            value == ""
            or looks_number
            or any(c in value for c in ":#{}[],&*!|>%@`'\"\n")
            or value.lower() in ("true", "false", "null", "yes", "no", "on", "off")
        ):
            return json.dumps(value, ensure_ascii=False)
        return value
    if isinstance(value, list):
        if not value:
            return "[]"
        lines = []
        for item in value:
            if isinstance(item, dict):
                nested = dump_yaml(item, indent + 1).splitlines()
                if not nested:
                    lines.append(sp + "- {}")
                    continue
                first = nested[0].lstrip()
                lines.append(sp + "- " + first)
                lines.extend(nested[1:])
            elif isinstance(item, list):
                lines.append(sp + "-")
                lines.append(dump_yaml(item, indent + 1))
            else:
                lines.append(sp + "- " + dump_yaml(item, 0))
        return "\n".join(lines)
    if isinstance(value, dict):
        if not value:
            return "{}"
        lines = []
        for k, v in value.items():
            key = str(k)
            if key.isdigit() or any(c in key for c in "[]:# "):
                key = json.dumps(key)
            if isinstance(v, dict):
                if not v:
                    lines.append(f"{sp}{key}: {{}}")
                else:
                    lines.append(f"{sp}{key}:")
                    lines.append(dump_yaml(v, indent + 1))
            elif isinstance(v, list):
                if not v:
                    lines.append(f"{sp}{key}: []")
                else:
                    lines.append(f"{sp}{key}:")
                    lines.append(dump_yaml(v, indent + 1))
            else:
                lines.append(f"{sp}{key}: {dump_yaml(v, 0)}")
        return "\n".join(lines)
    return json.dumps(value, ensure_ascii=False)


def main() -> None:
    if not SRC.is_file():
        sys.exit(f"missing {SRC}; run swag init first")
    oas2 = json.loads(SRC.read_text(encoding="utf-8"))
    if oas2.get("swagger") != "2.0" and "openapi" not in oas2:
        sys.exit("expected Swagger 2.0 JSON from swag")
    if str(oas2.get("openapi", "")).startswith("3."):
        oas2["openapi"] = "3.0.3"
        doc = oas2
    else:
        doc = to_oas3(oas2)
    DEST.write_text("# Generated by `make docs`. Do not edit.\n" + dump_yaml(doc) + "\n", encoding="utf-8")
    json_dest = ROOT / "docs" / "swagger" / "openapi.json"
    json_dest.write_text(json.dumps(doc, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    print(f"wrote {DEST.relative_to(ROOT)} and {json_dest.relative_to(ROOT)} (openapi {doc.get('openapi')})")


if __name__ == "__main__":
    main()
