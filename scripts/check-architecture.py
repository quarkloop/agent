#!/usr/bin/env python3
"""Validate Quark's package ownership import boundaries.

This is intentionally lightweight. It enforces the coarse ownership map in
architecture/ownership.json without trying to become a full dependency graph
tool. Deeper dependency checks can grow later, but this keeps the current
single-responsibility redlines executable.
"""

from __future__ import annotations

import ast
import json
import pathlib
import sys
from dataclasses import dataclass


ROOT = pathlib.Path(__file__).resolve().parents[1]
OWNERSHIP_FILE = ROOT / "architecture" / "ownership.json"


@dataclass(frozen=True)
class ImportFinding:
    area: str
    file: pathlib.Path
    import_path: str
    reason: str


def check_services_do_not_call_services() -> list[ImportFinding]:
    """Reject NATS client service-function invocation from service code.

    Services may host NATS responders and may use their owned JetStream stores,
    but only runtime/agents coordinate calls across service-function owners.
    """
    findings: list[ImportFinding] = []
    forbidden_tokens = (
        "ServiceOperation(",
        ".Call(",
        "OpenServiceStream(",
    )
    for file in go_files(ROOT / "services"):
        if file.name.endswith("_test.go"):
            continue
        source = file.read_text(encoding="utf-8")
        for token in forbidden_tokens:
            if token in source:
                findings.append(
                    ImportFinding(
                        "services",
                        file.relative_to(ROOT),
                        token,
                        "service code issues a service-function call; agents/runtime must coordinate",
                    )
                )
    return findings


def load_ownership() -> dict:
    with OWNERSHIP_FILE.open("r", encoding="utf-8") as handle:
        return json.load(handle)


def go_files(root: pathlib.Path) -> list[pathlib.Path]:
    ignored_parts = {
        ".git",
        "bin",
        "vendor",
    }
    out: list[pathlib.Path] = []
    for path in root.rglob("*.go"):
        rel_parts = set(path.relative_to(ROOT).parts)
        if ignored_parts & rel_parts:
            continue
        out.append(path)
    return sorted(out)


def imported_paths(path: pathlib.Path) -> list[str]:
    imports: list[str] = []
    lines = path.read_text(encoding="utf-8").splitlines()
    in_block = False
    for raw in lines:
        line = raw.strip()
        if line.startswith("import ("):
            in_block = True
            continue
        if in_block and line == ")":
            in_block = False
            continue
        if in_block:
            imports.extend(parse_import_line(line))
            continue
        if line.startswith("import "):
            imports.extend(parse_import_line(line[len("import ") :].strip()))
    return imports


def parse_import_line(line: str) -> list[str]:
    if not line or line.startswith("//"):
        return []
    if "//" in line:
        line = line.split("//", 1)[0].strip()
    try:
        value = ast.literal_eval(line.split()[-1])
    except (SyntaxError, ValueError):
        return []
    if isinstance(value, str):
        return [value]
    return []


def is_self_import(import_path: str, module: str) -> bool:
    return import_path == module or import_path.startswith(module + "/")


def load_exceptions(ownership: dict) -> set[tuple[str, str, str]]:
    exceptions: set[tuple[str, str, str]] = set()
    for item in ownership.get("known_import_exceptions", []):
        exceptions.add((item["area"], item["file"], item["import"]))
    return exceptions


def check_area(area: dict, exceptions: set[tuple[str, str, str]]) -> list[ImportFinding]:
    area_name = area["area"]
    area_root = ROOT / area["path"]
    module = area["module"]
    forbidden = tuple(area.get("forbidden_import_prefixes", []))
    findings: list[ImportFinding] = []

    for file in go_files(area_root):
        rel = file.relative_to(ROOT)
        for import_path in imported_paths(file):
            if is_self_import(import_path, module):
                continue
            if area.get("forbid_sibling_service_imports") and import_path.startswith("github.com/quarkloop/services/"):
                if (area_name, rel.as_posix(), import_path) not in exceptions:
                    findings.append(
                        ImportFinding(area_name, rel, import_path, "service imports another service module")
                    )
                continue
            for prefix in forbidden:
                if import_path == prefix or import_path.startswith(prefix):
                    if (area_name, rel.as_posix(), import_path) not in exceptions:
                        findings.append(
                            ImportFinding(area_name, rel, import_path, f"forbidden import prefix {prefix}")
                        )
                    break
    return findings


def main() -> int:
    ownership = load_ownership()
    exceptions = load_exceptions(ownership)
    findings: list[ImportFinding] = []
    for area in ownership["ownership"]:
        findings.extend(check_area(area, exceptions))
    findings.extend(check_services_do_not_call_services())

    if findings:
        print("architecture boundary violations:")
        for finding in findings:
            print(f"- {finding.area}: {finding.file}: {finding.import_path} ({finding.reason})")
        return 1

    if exceptions:
        print(f"architecture boundary checks passed with {len(exceptions)} documented exception(s)")
    else:
        print("architecture boundary checks passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
