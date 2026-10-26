"""Write public updater metadata before generating the signed checksum manifest."""
import argparse
import json
from pathlib import Path
import re


def write_metadata(directory: Path, repo: str, tag: str) -> None:
    if not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", repo):
        raise ValueError("invalid repository")
    if not re.fullmatch(r"v[0-9]+\.[0-9]+\.[0-9]+", tag):
        raise ValueError("expected stable release tag")
    root = f"https://github.com/{repo}/releases/"
    assets = []
    for path in sorted(directory.iterdir()):
        if not path.is_file() or path.name in {"release.json", "checksums.txt", "checksums.txt.sig"}:
            continue
        if not re.fullmatch(r"[A-Za-z0-9_.+-]+", path.name) or path.stat().st_size <= 0:
            raise ValueError(f"invalid release asset: {path.name}")
        assets.append({"name": path.name, "size": path.stat().st_size,
                       "browser_download_url": f"{root}download/{tag}/{path.name}"})
    if not assets:
        raise ValueError("no release assets")
    for name in ("checksums.txt", "checksums.txt.sig"):
        assets.append({"name": name, "size": 0,
                       "browser_download_url": f"{root}download/{tag}/{name}"})
    payload = {"tag_name": tag, "html_url": f"{root}tag/{tag}",
               "draft": False, "prerelease": False, "assets": assets}
    (directory / "release.json").write_text(json.dumps(payload, indent=2) + "\n", encoding="utf-8")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("directory", type=Path)
    parser.add_argument("repo")
    parser.add_argument("tag")
    args = parser.parse_args()
    write_metadata(args.directory, args.repo, args.tag)
