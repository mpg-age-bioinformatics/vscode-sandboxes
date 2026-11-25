#!/usr/bin/env python3
"""Prepare the sandbox-only Continue distribution before VS Code activates it."""
import gzip
import hashlib
import json
import os
from pathlib import Path
import platform
import shutil
import stat
import sys
import tempfile
import urllib.request
import zipfile

VERSION = "2.0.0"
REVISION = 3
ARCHIVES = {
    "linux-arm64": "42780285aaeb8e20caad8ed1816b4acc82718d76b3b91be9b8755403192759f0",
    "linux-x64": "e06893353f943dd4ed87d5438474d77ea850e6033abdf2c0694e42cb754c4c22",
}
MARKER = ".sandbox-continue-policy.json"
BACKEND_FROM = 'defaultToolPolicy: "allowedWithPermission"'
BACKEND_TO = BACKEND_FROM
GUI_FROM = 'const Xk="allowedWithPermission"'
GUI_TO = GUI_FROM
SEARCH_FROM = 'fetchSearchResults = async (query, n7, fetchFn) => {'
SEARCH_GUARD = '''
      // Sandbox-managed search guard. This service has no verified domain filter.
      const sandboxWebPolicy = JSON.parse(require("fs").readFileSync(require("path").join(require("os").homedir(), ".continue", ".sandbox-network.json"), "utf8"));
      if (sandboxWebPolicy.network !== "open") throw new Error("Hosted web search is disabled by sandbox network policy; fetch an allowed URL instead.");
'''


def digest(path):
    with open(path, "rb") as stream:
        checksum = hashlib.sha256()
        for chunk in iter(lambda: stream.read(1 << 20), b""):
            checksum.update(chunk)
        return checksum.hexdigest()


def patch_defaults(root):
    manifest_path = root / "package.json"
    manifest = json.loads(manifest_path.read_text())
    if (manifest.get("publisher", "").lower(), manifest.get("name"), manifest.get("version")) != ("continue", "continue", VERSION):
        raise ValueError("Unexpected Continue package; refusing to change its defaults")
    backend, gui = root / "out/extension.js", root / "gui/assets/index.js"
    backend_text, gui_text = backend.read_text(), gui.read_text()
    # Fail on version/schema drift rather than shipping a partially applied policy.
    if backend_text.count(BACKEND_FROM) != 10 or gui_text.count(GUI_FROM) != 1:
        raise ValueError("Continue policy layout changed; the sandbox defaults need review")
    # Preserve upstream approvals. Gate the shared hosted search implementation,
    # covering both search_web and @web, before any request reaches its service.
    if backend_text.count(SEARCH_FROM) != 1:
        raise ValueError("Continue web-search layout changed; refusing unfiltered search")
    backend.write_text(backend_text.replace(SEARCH_FROM, SEARCH_FROM + SEARCH_GUARD))
    manifest.setdefault("__metadata", {}).update({"pinned": True, "isMachineScoped": True})
    manifest_path.write_text(json.dumps(manifest, indent=2) + "\n")
    return {str(path.relative_to(root)): digest(path) for path in (manifest_path, backend, gui)}


def extract_bundle(archive, destination, target, expected):
    if digest(archive) != expected:
        raise ValueError("Continue package checksum mismatch")
    destination.mkdir(parents=True)
    with zipfile.ZipFile(archive) as package:
        for entry in package.infolist():
            if not entry.filename.startswith("extension/"):
                continue
            relative = Path(entry.filename).relative_to("extension")
            if relative.is_absolute() or ".." in relative.parts or stat.S_ISLNK(entry.external_attr >> 16):
                raise ValueError("Unsafe path in Continue package")
            output = destination / relative
            if entry.is_dir():
                output.mkdir(parents=True, exist_ok=True)
            else:
                output.parent.mkdir(parents=True, exist_ok=True)
                with package.open(entry) as source, open(output, "wb") as stream:
                    shutil.copyfileobj(source, stream)
                output.chmod(0o755 if entry.external_attr >> 16 & 0o111 else 0o644)
    hashes = patch_defaults(destination)
    metadata = json.loads((destination / "package.json").read_text())
    metadata["__metadata"]["targetPlatform"] = target
    (destination / "package.json").write_text(json.dumps(metadata, indent=2) + "\n")
    hashes["package.json"] = digest(destination / "package.json")
    (destination / MARKER).write_text(json.dumps({
        "revision": REVISION, "version": VERSION, "platform": target,
        "archive_sha256": expected, "files": hashes,
    }, indent=2) + "\n")


def valid_bundle(root, target):
    try:
        marker = json.loads((root / MARKER).read_text())
        return (marker["revision"] == REVISION and marker["version"] == VERSION
                and marker["platform"] == target and marker["archive_sha256"] == ARCHIVES[target]
                and all(digest(root / name) == expected for name, expected in marker["files"].items())
                and set(marker["files"]) == {"package.json", "out/extension.js", "gui/assets/index.js"})
    except (OSError, ValueError, KeyError):
        return False


def prepare():
    machine = platform.machine().lower()
    target = {"aarch64": "linux-arm64", "arm64": "linux-arm64", "x86_64": "linux-x64", "amd64": "linux-x64"}.get(machine)
    if sys.platform != "linux" or target is None:
        raise ValueError("Continue setup must run inside a supported Linux sandbox")
    cache = Path.home() / ".cache/vscode-sandboxes/continue" / VERSION / target
    name = f"continue.continue-{VERSION}-{target}"
    bundle = cache / "bundle" / name
    if valid_bundle(bundle, target):
        return bundle
    cache.mkdir(parents=True, exist_ok=True)
    archive = cache / "continue.vsix"
    if not archive.exists() or digest(archive) != ARCHIVES[target]:
        print(f"Downloading Continue {VERSION} ({target}) inside the sandbox…", file=sys.stderr)
        url = f"https://marketplace.visualstudio.com/_apis/public/gallery/publishers/Continue/vsextensions/continue/{VERSION}/vspackage?targetPlatform={target}"
        with tempfile.TemporaryFile() as download:
            with urllib.request.urlopen(url, timeout=120) as response:
                shutil.copyfileobj(response, download)
            download.seek(0)
            compressed = download.read(2) == b"\x1f\x8b"
            download.seek(0)
            with open(archive.with_suffix(".tmp"), "wb") as output:
                source = gzip.GzipFile(fileobj=download, mode="rb") if compressed else download
                shutil.copyfileobj(source, output)
        os.replace(archive.with_suffix(".tmp"), archive)
    with tempfile.TemporaryDirectory(dir=cache, prefix="prepare-") as temporary:
        staged = Path(temporary) / name
        extract_bundle(archive, staged, target, ARCHIVES[target])
        bundle.parent.mkdir(exist_ok=True)
        if bundle.exists():
            shutil.rmtree(bundle)
        os.replace(staged, bundle)
    return bundle


if __name__ == "__main__":
    try:
        print(prepare())
    except Exception as error:
        print(f"Continue setup failed: {error}", file=sys.stderr)
        sys.exit(1)
