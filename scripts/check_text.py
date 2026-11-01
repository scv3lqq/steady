"""Fail when a text file in the repository holds a literal em dash (U+2014).

The project keeps that character out of every source file; where the UI needs one it
builds it at runtime with String.fromCharCode(0x2014). A plain grep does not fit: the
needle would be spelled out in the check itself, grep options and locale handling
differ between macOS and Linux, and the file list must come from git so that ignored
paths (node_modules, bin, the database) are skipped. Here the needle is built from its
code point, and the files are whatever git tracks plus new files that are not ignored.
"""

import os
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
EM_DASH = chr(0x2014).encode()


def repo_files() -> list[Path]:
    listing = subprocess.run(
        ["git", "ls-files", "-z", "--cached", "--others", "--exclude-standard"],
        check=True,
        stdout=subprocess.PIPE,
        cwd=ROOT,
    ).stdout
    # An unmerged path is listed once per index stage.
    return sorted({Path(os.fsdecode(raw)) for raw in listing.split(b"\0") if raw})


def em_dash_lines(path: Path) -> list[int]:
    full = ROOT / path
    # Deleted in the work tree but still indexed, or a submodule directory.
    if not full.is_file():
        return []
    data = full.read_bytes()
    if b"\0" in data:
        return []
    return [n for n, line in enumerate(data.split(b"\n"), start=1) if EM_DASH in line]


def main() -> int:
    hits = [f"{path}:{n}" for path in repo_files() for n in em_dash_lines(path)]
    if not hits:
        return 0
    print("em dash (U+2014) found:")
    print("\n".join(hits))
    return 1


if __name__ == "__main__":
    sys.exit(main())
