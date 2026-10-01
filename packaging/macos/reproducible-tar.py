#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Create a tar.gz with stable ordering and metadata."""

from __future__ import annotations

import argparse
import gzip
import os
import tarfile
import tempfile
from collections.abc import Iterator
from pathlib import Path


def source_paths(root: Path) -> Iterator[Path]:
    yield root
    if root.is_symlink() or not root.is_dir():
        return
    for child in sorted(root.iterdir(), key=lambda path: path.name.encode("utf-8")):
        yield from source_paths(child)


def normalized_mode(info: tarfile.TarInfo) -> int:
    if info.isdir():
        return 0o755
    if info.issym():
        return 0o777
    if info.isfile():
        return 0o755 if info.mode & 0o111 else 0o644
    raise ValueError(f"unsupported archive entry type: {info.name}")


def write_archive(source: Path, output: Path, mtime: int) -> None:
    if mtime < 0 or mtime > 0xFFFFFFFF:
        raise ValueError("mtime is outside the portable gzip range")

    source = source.resolve(strict=True)
    if not source.is_dir():
        raise ValueError(f"source is not a directory: {source}")

    output.parent.mkdir(parents=True, exist_ok=True)
    temporary = tempfile.NamedTemporaryFile(
        dir=output.parent, prefix=f".{output.name}.", suffix=".tmp", delete=False
    )
    temporary_path = Path(temporary.name)
    temporary.close()

    try:
        with temporary_path.open("wb") as raw_output:
            with gzip.GzipFile(
                filename="",
                mode="wb",
                fileobj=raw_output,
                compresslevel=9,
                mtime=mtime,
            ) as compressed:
                with tarfile.open(
                    fileobj=compressed, mode="w", format=tarfile.PAX_FORMAT
                ) as archive:
                    for path in source_paths(source):
                        relative = path.relative_to(source.parent).as_posix()
                        info = archive.gettarinfo(str(path), arcname=relative)
                        info.uid = 0
                        info.gid = 0
                        info.uname = ""
                        info.gname = ""
                        info.mtime = mtime
                        info.mode = normalized_mode(info)
                        info.pax_headers = {}
                        if info.isfile():
                            with path.open("rb") as payload:
                                archive.addfile(info, payload)
                        else:
                            archive.addfile(info)

        os.chmod(temporary_path, 0o644)
        os.utime(temporary_path, (mtime, mtime))
        os.replace(temporary_path, output)
    finally:
        temporary_path.unlink(missing_ok=True)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--source", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--mtime", required=True, type=int)
    args = parser.parse_args()
    write_archive(args.source, args.output, args.mtime)


if __name__ == "__main__":
    main()
