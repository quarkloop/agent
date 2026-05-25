#!/usr/bin/env python3
"""Reject legacy standalone embedding/model service traces.

Gateway is Quark's only production provider and embedding boundary. Indexer
storage predicates and Gateway embedding vocabulary are intentional and are
therefore outside this narrow regression check.
"""

from __future__ import annotations

import pathlib
import subprocess
import sys


ROOT = pathlib.Path(__file__).resolve().parents[1]

FORBIDDEN_PATH_PREFIXES = (
    "services/embedding/",
    "services/model/",
    "plugins/services/embedding/",
    "plugins/services/embedding-openrouter/",
    "plugins/services/model/",
    "proto/quark/embedding/",
    "proto/quark/model/",
    "pkg/serviceapi/gen/quark/embedding/",
    "pkg/serviceapi/gen/quark/model/",
)

FORBIDDEN_TERMS = (
    "embedding_Embed",
    "embedding.*",
    "quark.embedding.v1",
    "svc.embedding.",
    "QUARK_EMBEDDING_ADDR",
    "QUARK_EMBEDDING_SERVICE",
    "QUARK_EMBEDDING_NATS",
)

SCANNED_SUFFIXES = {
    ".go",
    ".json",
    ".md",
    ".proto",
    ".py",
    ".yaml",
    ".yml",
}

SCANNED_TOP_LEVEL_FILES = {
    "Makefile",
    ".env.example",
}


def tracked_files() -> list[str]:
    output = subprocess.check_output(
        ["git", "ls-files", "--cached", "--others", "--exclude-standard"],
        cwd=ROOT,
        text=True,
    )
    return [line.strip() for line in output.splitlines() if line.strip()]


def main() -> int:
    violations: list[str] = []
    files = tracked_files()
    for path in files:
        if path.startswith(FORBIDDEN_PATH_PREFIXES):
            violations.append(f"legacy path remains tracked: {path}")

    for path in files:
        relative = pathlib.PurePosixPath(path)
        if relative.suffix not in SCANNED_SUFFIXES and path not in SCANNED_TOP_LEVEL_FILES:
            continue
        if path == "scripts/check-legacy-embedding.py":
            continue
        source = ROOT / path
        if not source.exists():
            continue
        data = source.read_text(encoding="utf-8", errors="replace")
        for line_number, line in enumerate(data.splitlines(), start=1):
            for term in FORBIDDEN_TERMS:
                if term in line:
                    violations.append(f"legacy identifier {term!r}: {path}:{line_number}")

    if violations:
        print("legacy embedding boundary violations:")
        for violation in violations:
            print(f"- {violation}")
        return 1
    print("legacy embedding boundary check passed: Gateway is the sole tracked embedding boundary")
    return 0


if __name__ == "__main__":
    sys.exit(main())
