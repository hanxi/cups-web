#!/usr/bin/python3
"""Prepare, install and restore the version-matched HP vendor plug-in."""

import configparser
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
import zipfile


SOURCE = "https://www.openprinting.org/download/printdriver/auxfiles/HP/plugins"
DRIVERS = Path("/opt/cups-drivers/data/hplip-plugin")
RUNTIME = Path("/run/cups-web/hplip-plugin")
STATE = Path("/var/lib/hp/hplip.state")
MAX_PACKAGE = 64 * 1024 * 1024
MEMBERS = {"plugin.run": MAX_PACKAGE, "plugin.asc": 64 * 1024}
ARCH_SUFFIX = {"amd64": "x86_64", "arm64": "arm64", "armhf": "arm32"}


def read_ini(path):
    config = configparser.ConfigParser(interpolation=None)
    config.read(path)
    return config


def hplip_config():
    config = read_ini("/etc/hp/hplip.conf")
    version = config.get("hplip", "version", fallback="")
    if not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+[a-z]?", version):
        raise ValueError("Cannot determine the installed HPLIP version")
    arch = subprocess.check_output(["dpkg", "--print-architecture"], text=True).strip()
    home = Path(config["dirs"]["home"])
    return version, arch, home


def is_installed(version, home):
    try:
        state = read_ini(STATE)
    except (OSError, configparser.Error):
        return False
    return (
        state.get("plugin", "version", fallback="") == version
        and state.get("plugin", "installed", fallback="") == "1"
        and state.get("plugin", "eula", fallback="") == "1"
        and (home / "plugin.spec").is_file()
        and any(path.is_file() for path in (home / "scan/plugins").glob("*.so"))
    )


def verify_and_extract(work, version, arch, home):
    gnupg = work / "gnupg"
    gnupg.mkdir(mode=0o700)
    keyring = gnupg / "hplip.gpg"
    gpg = ["gpg", "--no-options", "--batch", "--homedir", str(gnupg)]
    # Debian ships the key matching its HPLIP release. Never fetch trust material
    # from the same server as the installer or continue after a failed check.
    subprocess.run(gpg + ["--dearmor", "--output", str(keyring), str(home / "signing-key.asc")],
                   stdout=sys.stderr, check=True, timeout=15)
    verification = subprocess.run(
        gpg + ["--no-default-keyring", "--keyring", str(keyring),
               "--trust-model", "always", "--no-auto-key-retrieve", "--verify",
               str(work / "plugin.asc"), str(work / "plugin.run")],
        capture_output=True, text=True, timeout=30,
    )
    if verification.returncode != 0:
        sys.stderr.write(verification.stderr)
        raise ValueError("HP package signature verification failed")
    print("[hplip-plugin] HP package signature verified", file=sys.stderr)

    files = work / "files"
    subprocess.run(["sh", str(work / "plugin.run"), "--noexec", "--quiet", "--target", str(files)],
                   cwd=work, stdout=sys.stderr, check=True, timeout=30)
    if (files / "version.txt").read_text().strip() != version:
        raise ValueError("HP plug-in version does not match HPLIP")
    if not any(files.glob("*-" + ARCH_SUFFIX[arch] + ".so*")):
        raise ValueError("HP plug-in contains no libraries for " + arch)
    return files


def write_bundle(work, target):
    target.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    fd, staging = tempfile.mkstemp(prefix=".plugin-", dir=target.parent)
    try:
        with os.fdopen(fd, "wb") as output, zipfile.ZipFile(output, "w") as bundle:
            for name in MEMBERS:
                bundle.write(work / name, name)
        os.replace(staging, target)
    finally:
        Path(staging).unlink(missing_ok=True)


def read_bundle(source, work):
    with zipfile.ZipFile(source) as bundle:
        for name, limit in MEMBERS.items():
            member = bundle.getinfo(name)
            if not 0 < member.file_size <= limit:
                raise ValueError("Invalid HP plug-in archive member: " + name)
            with bundle.open(member) as entry, (work / name).open("wb") as output:
                shutil.copyfileobj(entry, output)


def digest(work):
    with (work / "plugin.run").open("rb") as payload:
        return hashlib.file_digest(payload, "sha256").hexdigest()


def prepare(work, pending, version, arch, home):
    url = f"{SOURCE}/hplip-{version}-plugin.run"
    for name, limit in MEMBERS.items():
        endpoint = url if name == "plugin.run" else url + ".asc"
        subprocess.run(["curl", "--fail", "--silent", "--show-error", "--location",
                        "--proto", "=https", "--proto-redir", "=https", "--connect-timeout", "15",
                        "--max-time", "360", "--max-filesize", str(limit),
                        "--output", str(work / name), endpoint], check=True)
    files = verify_and_extract(work, version, arch, home)
    with (files / "license.txt").open(encoding="utf-8") as source:
        license_text = source.read(65537)
    if not license_text.strip() or len(license_text) > 65536:
        raise ValueError("Invalid HP plug-in license")
    write_bundle(work, pending)
    print(json.dumps({"version": version, "license": license_text, "digest": digest(work)}))


def install(files, version, home):
    # The front end displays this signed bundle's license before installation.
    # Use the vendor's install step after acceptance, including for offline restore.
    STATE.parent.mkdir(parents=True, exist_ok=True)
    subprocess.run([sys.executable, "installPlugin.py"], cwd=files, check=True, timeout=90)
    if not is_installed(version, home):
        raise ValueError("HPLIP did not install the plug-in libraries")


def main():
    action = sys.argv[1]
    if action == "restore" and not DRIVERS.is_dir():
        return
    version, arch, home = hplip_config()
    supported = arch in ARCH_SUFFIX
    archive = DRIVERS / arch / (version + ".zip")
    if action == "status":
        print(json.dumps({"version": version, "supported": supported,
                          "installed": is_installed(version, home), "saved": archive.is_file()}))
        return
    if not supported:
        raise ValueError("Unsupported HPLIP plug-in architecture: " + arch)
    if action == "restore":
        if not archive.is_file():
            print(f"[hplip-plugin] No saved plug-in for HPLIP {version} ({arch}). "
                  "Install the matching plug-in in Driver Management.", file=sys.stderr)
            return
        if is_installed(version, home):
            return

    RUNTIME.mkdir(mode=0o700, parents=True, exist_ok=True)
    pending = RUNTIME / "pending.zip"
    with tempfile.TemporaryDirectory(dir=RUNTIME) as folder:
        work = Path(folder)
        if action == "prepare":
            prepare(work, pending, version, arch, home)
            return
        if action == "install":
            if not pending.is_file():
                raise ValueError("Prepare the HPLIP plug-in and review its license again")
            read_bundle(pending, work)
            if digest(work) != sys.argv[2]:
                raise ValueError("The prepared HPLIP plug-in has changed. Review its license again")
        elif action == "restore":
            read_bundle(archive, work)
        else:
            raise ValueError("Unknown HPLIP plug-in action")
        files = verify_and_extract(work, version, arch, home)
        install(files, version, home)
        if action == "install":
            write_bundle(work, archive)
            pending.unlink(missing_ok=True)
    print(f"[hplip-plugin] HPLIP {version} plug-in ready for {arch}")


if __name__ == "__main__":
    try:
        main()
    except (OSError, KeyError, ValueError, configparser.Error,
            subprocess.SubprocessError, zipfile.BadZipFile) as exc:
        sys.exit(f"[hplip-plugin] {exc}")
