#!/usr/bin/env python3
"""Build Quark's service implementation inventory.

The inventory is intentionally generated from source-of-truth files instead of
hand-maintained prose. It reconciles service plugin manifests, protobuf APIs,
runnable service modules, agent profile permissions, and E2E tests so missing
production wiring stays visible while the service platform is being completed.
"""

from __future__ import annotations

import argparse
import json
import pathlib
import re
import sys
from dataclasses import dataclass
from typing import Any


ROOT = pathlib.Path(__file__).resolve().parents[1]
OUTPUT = ROOT / "architecture" / "service-implementation-map.json"
SUBJECT_OUTPUT = ROOT / "architecture" / "nats-subjects.md"

AREA_SERVICES = {
    "Knowledge": ["document", "runstate", "gateway", "indexer", "citation", "harness"],
    "DevOps": ["devops"],
    "System": ["system"],
    "Core": ["core"],
    "Gateway": ["gateway"],
    "Space": ["space"],
    "Indexer": ["indexer"],
}

AGENT_AREA = {
    "quark-knowledge": "Knowledge",
    "quark-devops": "DevOps",
    "quark-system": "System",
}

PROMPT_SHORTCUT_TERMS = {
    "approval_required",
    "gateway_Embed",
    "indexer_DeleteChunk",
    "indexer_QueryContext",
    "indexer_UpsertChunk",
    "service function",
    "tool call",
}

DIRECT_SHORTCUT_TERMS = {
    ".DeleteChunk(",
    ".Embed(",
    ".QueryContext(",
    ".UpsertChunk(",
}


@dataclass(frozen=True)
class ProtoRPC:
    service: str
    method: str
    request: str
    response: str


def nats_token(value: str) -> str:
    value = value.strip()
    out: list[str] = []
    previous_lower_or_digit = False
    for char in value:
        if char.isupper() and previous_lower_or_digit:
            out.append("_")
        if char.isalnum():
            out.append(char.lower())
            previous_lower_or_digit = char.islower() or char.isdigit()
        else:
            if out and out[-1] != "_":
                out.append("_")
            previous_lower_or_digit = False
    return re.sub(r"_+", "_", "".join(out)).strip("_")


def service_subject(owner: str, function_name: str) -> str:
    owner_token = nats_token(owner)
    function = function_name.strip()
    for prefix in (owner + "_", owner_token + "_"):
        if function.startswith(prefix):
            function = function[len(prefix):]
            break
    return f"svc.{owner_token}.v1.{nats_token(function)}"


def parse_scalar(raw: str) -> Any:
    value = raw.strip()
    if value.startswith('"') and value.endswith('"'):
        return value[1:-1]
    if value in {"true", "false"}:
        return value == "true"
    return value


def scalar_after_colon(line: str) -> tuple[str, Any] | None:
    if ":" not in line:
        return None
    key, raw_value = line.split(":", 1)
    key = key.strip()
    raw_value = raw_value.strip()
    if not key:
        return None
    if raw_value == "":
        return key, None
    return key, parse_scalar(raw_value)


def parse_service_manifest(path: pathlib.Path) -> dict[str, Any]:
    top: dict[str, Any] = {}
    service: dict[str, Any] = {
        "address_env": "",
        "health_service": "",
        "readiness_required": False,
        "skill": "",
        "readme": "",
        "proto_services": [],
        "functions": [],
    }
    state = ""
    current_function: dict[str, Any] | None = None
    in_service = False
    in_health = False
    in_readiness = False
    in_approval_requirements = False

    for raw in path.read_text(encoding="utf-8").splitlines():
        if not raw.strip() or raw.lstrip().startswith("#"):
            continue
        indent = len(raw) - len(raw.lstrip(" "))
        line = raw.strip()

        if indent == 0:
            in_service = line == "service:"
            in_health = False
            in_readiness = False
            state = ""
            parsed = scalar_after_colon(line)
            if parsed and parsed[1] is not None:
                top[parsed[0]] = parsed[1]
            continue

        if not in_service:
            parsed = scalar_after_colon(line)
            if indent == 0 and parsed and parsed[1] is not None:
                top[parsed[0]] = parsed[1]
            continue

        if line == "health:":
            in_health = True
            in_readiness = False
            continue
        if line == "readiness:":
            in_health = False
            in_readiness = True
            continue
        if line == "proto_services:":
            state = "proto_services"
            in_health = False
            in_readiness = False
            continue
        if line == "functions:":
            state = "functions"
            in_health = False
            in_readiness = False
            continue

        if state == "proto_services" and line.startswith("- "):
            service["proto_services"].append(line[2:].strip())
            continue

        if state == "functions":
            if line.startswith("- name:"):
                current_function = {"name": parse_scalar(line.split(":", 1)[1])}
                service["functions"].append(current_function)
                in_approval_requirements = False
                continue
            if current_function is None:
                continue
            if line == "approval_requirements:":
                current_function["approval_requirements"] = []
                in_approval_requirements = True
                continue
            if in_approval_requirements and line.startswith("- "):
                current_function.setdefault("approval_requirements", []).append(line[2:].strip())
                continue
            parsed = scalar_after_colon(line)
            if parsed and parsed[1] is not None:
                current_function[parsed[0]] = parsed[1]
            continue

        parsed = scalar_after_colon(line)
        if not parsed or parsed[1] is None:
            continue
        key, value = parsed
        if in_health and key == "service":
            service["health_service"] = value
        elif in_readiness and key == "required":
            service["readiness_required"] = value
        else:
            service[key] = value

    return {
        "name": top.get("name", path.parent.name),
        "version": top.get("version", ""),
        "type": top.get("type", ""),
        "mode": top.get("mode", ""),
        "manifest_path": path.relative_to(ROOT).as_posix(),
        "skill_path": (path.parent / service["skill"]).relative_to(ROOT).as_posix() if service["skill"] else "",
        "readme_path": (path.parent / service["readme"]).relative_to(ROOT).as_posix() if service["readme"] else "",
        "address_env": service["address_env"],
        "health_service": service["health_service"],
        "readiness_required": service["readiness_required"],
        "proto_services": service["proto_services"],
        "functions": service["functions"],
    }


def parse_agent_profile(path: pathlib.Path) -> dict[str, Any]:
    profile: dict[str, Any] = {
        "id": path.parent.name,
        "name": path.parent.name,
        "path": path.relative_to(ROOT).as_posix(),
        "service_permissions": [],
    }
    in_permissions = False
    in_services = False

    for raw in path.read_text(encoding="utf-8").splitlines():
        if not raw.strip() or raw.lstrip().startswith("#"):
            continue
        indent = len(raw) - len(raw.lstrip(" "))
        line = raw.strip()
        parsed = scalar_after_colon(line)
        if indent == 0:
            in_permissions = line == "permissions:"
            in_services = False
            if parsed and parsed[1] is not None and parsed[0] in {"id", "name", "description"}:
                profile[parsed[0]] = parsed[1]
            continue
        if in_permissions and indent == 2:
            in_services = line == "services:"
            continue
        if in_services and line.startswith("- "):
            profile["service_permissions"].append(line[2:].strip())
            continue
        if in_services and indent <= 2 and not line.startswith("- "):
            in_services = False

    return profile


def parse_proto_files() -> dict[str, dict[str, Any]]:
    services: dict[str, dict[str, Any]] = {}
    rpc_re = re.compile(r"rpc\s+(\w+)\s*\(([^)]*)\)\s+returns\s+\(([^)]*)\)")

    for path in sorted((ROOT / "proto" / "quark").rglob("*.proto")):
        package = ""
        current_service = ""
        for raw in path.read_text(encoding="utf-8").splitlines():
            line = raw.strip()
            if line.startswith("package "):
                package = line.removeprefix("package ").removesuffix(";").strip()
                continue
            if line.startswith("service "):
                current_service = line.split()[1]
                full_name = f"{package}.{current_service}" if package else current_service
                services.setdefault(
                    full_name,
                    {
                        "name": full_name,
                        "proto_path": path.relative_to(ROOT).as_posix(),
                        "rpcs": {},
                    },
                )
                continue
            if current_service:
                match = rpc_re.search(line)
                if match:
                    full_name = f"{package}.{current_service}" if package else current_service
                    services[full_name]["rpcs"][match.group(1)] = {
                        "request": match.group(2).strip(),
                        "response": match.group(3).strip(),
                    }
                if line == "}":
                    current_service = ""

    return services


def service_module_for(manifest: dict[str, Any]) -> pathlib.Path | None:
    by_name = ROOT / "services" / manifest["name"]
    if (by_name / "go.mod").exists() or (by_name / "Cargo.toml").exists():
        return by_name
    for proto_service in manifest["proto_services"]:
        parts = proto_service.split(".")
        if len(parts) >= 2:
            candidate = ROOT / "services" / parts[1]
            if (candidate / "go.mod").exists() or (candidate / "Cargo.toml").exists():
                return candidate
    return None


def scan_service_method(module: pathlib.Path | None, method: str) -> bool:
    if module is None:
        return False
    if (module / "go.mod").exists():
        method_re = re.compile(rf"func\s+\([^)]*\)\s+{re.escape(method)}\s*\(")
        for path in sorted(module.rglob("*.go")):
            if path.name.endswith("_test.go"):
                continue
            if method_re.search(path.read_text(encoding="utf-8")):
                return True
    if (module / "Cargo.toml").exists():
        rust_operation = re.compile(rf"Operation::{re.escape(method)}\b")
        rust_method = re.compile(rf"\bfn\s+{re.escape(nats_token(method))}\s*\(")
        for path in sorted((module / "src").rglob("*.rs")):
            text = path.read_text(encoding="utf-8")
            if rust_operation.search(text) or rust_method.search(text):
                return True
    return False


def build_service_inventory(manifests: list[dict[str, Any]], proto_services: dict[str, dict[str, Any]]) -> dict[str, Any]:
    out: dict[str, Any] = {}
    for manifest in manifests:
        module = service_module_for(manifest)
        functions: list[dict[str, Any]] = []
        for fn in manifest["functions"]:
            owner = str(fn.get("owner", manifest["name"]))
            service_name = str(fn.get("service", ""))
            method = str(fn.get("method", ""))
            proto_rpc = proto_services.get(service_name, {}).get("rpcs", {}).get(method)
            method_implemented = scan_service_method(module, method)
            status = "implemented_end_to_end" if module and proto_rpc and method_implemented else "declared_only"
            if not proto_rpc:
                status = "proto_mismatch"
            functions.append(
                {
                    "name": fn.get("name", ""),
                    "owner": owner,
                    "subject": fn.get("subject", "") or service_subject(owner, str(fn.get("name", ""))),
                    "service": service_name,
                    "method": method,
                    "request": fn.get("request", ""),
                    "response": fn.get("response", ""),
                    "risk_level": fn.get("risk_level", ""),
                    "approval_required": bool(fn.get("approval_required", False)),
                    "idempotent": bool(fn.get("idempotent", False)),
                    "streaming": bool(fn.get("streaming", False)),
                    "proto_rpc_exists": bool(proto_rpc),
                    "method_implemented": method_implemented,
                    "status": status,
                }
            )
        out[manifest["name"]] = {
            "manifest_path": manifest["manifest_path"],
            "skill_path": manifest["skill_path"],
            "readme_path": manifest["readme_path"],
            "module_path": module.relative_to(ROOT).as_posix() if module else "",
            "module_exists": module is not None,
            "address_env": manifest["address_env"],
            "health_service": manifest["health_service"],
            "readiness_required": manifest["readiness_required"],
            "proto_services": manifest["proto_services"],
            "function_count": len(functions),
            "implemented_function_count": sum(1 for fn in functions if fn["status"] == "implemented_end_to_end"),
            "declared_only_function_count": sum(1 for fn in functions if fn["status"] == "declared_only"),
            "proto_mismatch_count": sum(1 for fn in functions if fn["status"] == "proto_mismatch"),
            "functions": functions,
        }
    return dict(sorted(out.items()))


def expand_permission(permission: str, service_functions: dict[str, dict[str, Any]]) -> tuple[list[str], bool]:
    if permission.endswith(".*"):
        prefix = permission[:-2] + "_"
        matches = [name for name in service_functions if name.startswith(prefix)]
        return sorted(matches), bool(matches)
    return [permission] if permission in service_functions else [], permission in service_functions


def build_profile_inventory(profiles: list[dict[str, Any]], services: dict[str, dict[str, Any]]) -> dict[str, Any]:
    all_functions = {
        fn["name"]: fn
        for service in services.values()
        for fn in service["functions"]
        if fn.get("name")
    }
    profiles_out: dict[str, Any] = {}
    for profile in profiles:
        missing: list[str] = []
        declared_only: list[str] = []
        implemented: list[str] = []
        for permission in profile["service_permissions"]:
            matches, exists = expand_permission(permission, all_functions)
            if not exists:
                missing.append(permission)
                continue
            for fn_name in matches:
                status = all_functions[fn_name]["status"]
                if status == "implemented_end_to_end":
                    implemented.append(fn_name)
                else:
                    declared_only.append(fn_name)
        profiles_out[profile["id"]] = {
            "name": profile["name"],
            "path": profile["path"],
            "area": AGENT_AREA.get(profile["id"], ""),
            "service_permission_count": len(profile["service_permissions"]),
            "implemented_permissions": sorted(set(implemented)),
            "declared_only_permissions": sorted(set(declared_only)),
            "missing_permissions": sorted(set(missing)),
        }
    return dict(sorted(profiles_out.items()))


def scan_e2e_tests() -> dict[str, Any]:
    files: list[dict[str, Any]] = []
    for path in sorted((ROOT / "e2e").rglob("*_test.go")):
        rel = path.relative_to(ROOT).as_posix()
        text = path.read_text(encoding="utf-8")
        prompt_shortcuts: list[str] = []
        direct_shortcuts: list[str] = []
        for idx, raw in enumerate(text.splitlines(), start=1):
            line = raw.strip()
            if any(term in line for term in PROMPT_SHORTCUT_TERMS):
                if '"' in line or "`" in line:
                    prompt_shortcuts.append(f"{idx}: {line[:180]}")
            if any(term in line for term in DIRECT_SHORTCUT_TERMS):
                direct_shortcuts.append(f"{idx}: {line[:180]}")
        files.append(
            {
                "path": rel,
                "prompt_shortcut_suspects": prompt_shortcuts,
                "direct_service_shortcut_suspects": direct_shortcuts,
                "has_process_flow": "StartProcess" in text or "StartSupervisor" in text,
            }
        )
    return {
        "file_count": len(files),
        "prompt_shortcut_suspect_count": sum(len(item["prompt_shortcut_suspects"]) for item in files),
        "direct_service_shortcut_suspect_count": sum(len(item["direct_service_shortcut_suspects"]) for item in files),
        "files": files,
    }


def build_area_map(services: dict[str, dict[str, Any]], profiles: dict[str, dict[str, Any]]) -> dict[str, Any]:
    areas: dict[str, Any] = {}
    for area, service_names in AREA_SERVICES.items():
        area_services = [services[name] for name in service_names if name in services]
        area_profiles = [profile for profile in profiles.values() if profile["area"] == area]
        areas[area] = {
            "services": service_names,
            "profile_count": len(area_profiles),
            "service_count": len(area_services),
            "runnable_service_count": sum(1 for service in area_services if service["module_exists"]),
            "implemented_function_count": sum(service["implemented_function_count"] for service in area_services),
            "declared_only_function_count": sum(service["declared_only_function_count"] for service in area_services),
            "proto_mismatch_count": sum(service["proto_mismatch_count"] for service in area_services),
        }
    return areas


def build_inventory() -> dict[str, Any]:
    manifests = [parse_service_manifest(path) for path in sorted((ROOT / "plugins" / "services").glob("*/manifest.yaml"))]
    profiles = [parse_agent_profile(path) for path in sorted((ROOT / "plugins" / "agents").glob("*/PROFILE.yaml"))]
    proto_services = parse_proto_files()
    services = build_service_inventory(manifests, proto_services)
    profile_inventory = build_profile_inventory(profiles, services)
    e2e = scan_e2e_tests()
    declared_only = [
        fn["name"]
        for service in services.values()
        for fn in service["functions"]
        if fn["status"] != "implemented_end_to_end"
    ]
    return {
        "version": 1,
        "summary": {
            "service_plugin_count": len(services),
            "runnable_service_module_count": sum(1 for item in services.values() if item["module_exists"]),
            "service_function_count": sum(item["function_count"] for item in services.values()),
            "implemented_service_function_count": sum(item["implemented_function_count"] for item in services.values()),
            "declared_only_or_mismatched_function_count": len(declared_only),
            "agent_profile_count": len(profile_inventory),
            "e2e_test_file_count": e2e["file_count"],
        },
        "areas": build_area_map(services, profile_inventory),
        "services": services,
        "agent_profiles": profile_inventory,
        "e2e_tests": e2e,
    }


def write_if_changed(path: pathlib.Path, content: str) -> bool:
    if path.exists() and path.read_text(encoding="utf-8") == content:
        return False
    path.write_text(content, encoding="utf-8")
    return True


def render_subject_catalog(inventory: dict[str, Any]) -> str:
    lines = [
        "# NATS Subject Catalog",
        "",
        "This file is generated by `scripts/service_inventory.py --write` from service plugin manifests.",
        "Do not edit it manually.",
        "",
        "## Service Function Routing",
        "",
        "Service requests use Core NATS request/reply subjects in the form",
        "`svc.<owner>.v1.<function>`. Service hosts derive the responder queue",
        "group as `q.service.v1.<owner>` through `pkg/natskit`; callers never",
        "select queue groups.",
        "",
        "| Owner | Responder queue group | Function | Request subject |",
        "| --- | --- | --- | --- |",
    ]
    for owner, service in sorted(inventory["services"].items()):
        queue = f"q.service.v1.{nats_token(owner)}"
        for function in service["functions"]:
            lines.append(f"| `{owner}` | `{queue}` | `{function['name']}` | `{function['subject']}` |")
    lines.extend(
        [
            "",
            "## Other Owned Subject Families",
            "",
            "| Owner | Family | Contract source |",
            "| --- | --- | --- |",
            "| Supervisor control plane | `control.<resource>.v1.<operation>` | `pkg/serviceapi/clientcontract/subjects.go` |",
            "| Supervisor catalog | `catalog.runtime.v1.<operation>` | `pkg/serviceapi/clientcontract/subjects.go` |",
            "| Runtime session channel | `session.<session_id>.<operation>` | `pkg/serviceapi/clientcontract/subjects.go` |",
            "| Runtime inspection | `runtime.<resource>.v1.<operation>` | `pkg/serviceapi/clientcontract/subjects.go` |",
            "| Service-call audit records | JetStream subjects derived by `pkg/natskit` | `pkg/natskit/audit.go` |",
            "",
        ]
    )
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--write", action="store_true", help=f"write {OUTPUT.relative_to(ROOT)}")
    parser.add_argument("--check", action="store_true", help=f"verify {OUTPUT.relative_to(ROOT)} is up to date")
    args = parser.parse_args()

    inventory = build_inventory()
    rendered = json.dumps(inventory, indent=2, sort_keys=True) + "\n"
    subject_rendered = render_subject_catalog(inventory)

    if args.check:
        for output, content in ((OUTPUT, rendered), (SUBJECT_OUTPUT, subject_rendered)):
            if not output.exists():
                print(f"{output.relative_to(ROOT)} is missing")
                return 1
            if output.read_text(encoding="utf-8") != content:
                print(f"{output.relative_to(ROOT)} is out of date; run scripts/service_inventory.py --write")
                return 1
        print("service implementation inventory is up to date")
        return 0

    if args.write:
        for output, content in ((OUTPUT, rendered), (SUBJECT_OUTPUT, subject_rendered)):
            changed = write_if_changed(output, content)
            action = "updated" if changed else "unchanged"
            print(f"{action}: {output.relative_to(ROOT)}")
        return 0

    print(rendered, end="")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
