#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Create a ZIP with stable ordering and metadata."""

from __future__ import annotations

import argparse
import datetime as dt
import os
import shutil
import stat
import tempfile
import zipfile
from pathlib import Path

ZIP_MIN_EPOCH = 315532800  # 1980-01-01, the minimum representable ZIP date.
ZIP_MAX_EPOCH = 4354819198  # 2107-12-31 23:59:58 UTC.


def archive_paths(root: Path) -> list[Path]:
    paths = [root, *root.rglob("*")]
    return sorted(
        paths, key=lambda path: path.relative_to(root.parent).as_posix().encode("utf-8")
    )


def zip_info(name: str, path: Path, timestamp: tuple[int, ...]) -> zipfile.ZipInfo:
    is_directory = path.is_dir()
    if path.is_symlink():
        raise ValueError(f"symbolic links are not supported in Windows ZIPs: {path}")

    entry_name = name.rstrip("/") + "/" if is_directory else name
    info = zipfile.ZipInfo(entry_name, date_time=timestamp[:6])
    info.create_system = 3
    info.compress_type = zipfile.ZIP_STORED if is_directory else zipfile.ZIP_DEFLATED
    info.extra = b""
    info.comment = b""
    if is_directory:
        info.external_attr = (stat.S_IFDIR | 0o755) << 16 | 0x10
    else:
        file_stat = path.stat()
        info.file_size = file_stat.st_size
        mode = 0o755 if file_stat.st_mode & 0o111 else 0o644
        info.external_attr = (stat.S_IFREG | mode) << 16
    return info


def write_archive(source: Path, output: Path, mtime: int) -> None:
    if mtime < 0 or mtime > ZIP_MAX_EPOCH:
        raise ValueError("mtime is outside the portable ZIP range")

    source = source.resolve(strict=True)
    if not source.is_dir():
        raise ValueError(f"source is not a directory: {source}")

    zip_epoch = max(mtime, ZIP_MIN_EPOCH)
    timestamp = dt.datetime.fromtimestamp(zip_epoch, tz=dt.timezone.utc).timetuple()
    output.parent.mkdir(parents=True, exist_ok=True)
    temporary = tempfile.NamedTemporaryFile(
        dir=output.parent, prefix=f".{output.name}.", suffix=".tmp", delete=False
    )
    temporary_path = Path(temporary.name)
    temporary.close()

    try:
        with zipfile.ZipFile(
            temporary_path,
            mode="w",
            compression=zipfile.ZIP_DEFLATED,
            compresslevel=9,
            strict_timestamps=True,
        ) as archive:
            for path in archive_paths(source):
                name = path.relative_to(source.parent).as_posix()
                info = zip_info(name, path, timestamp)
                if path.is_dir():
                    archive.writestr(info, b"")
                else:
                    with path.open("rb") as payload:
                        force_zip64 = info.file_size >= zipfile.ZIP64_LIMIT
                        with archive.open(
                            info, mode="w", force_zip64=force_zip64
                        ) as target:
                            shutil.copyfileobj(payload, target, length=1024 * 1024)

        os.chmod(temporary_path, 0o644)
        os.utime(temporary_path, (zip_epoch, zip_epoch))
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
