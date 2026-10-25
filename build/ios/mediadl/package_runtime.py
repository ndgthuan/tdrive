"""Stage BeeWare CPython, native stdlib modules and yt-dlp into an iOS app."""
import argparse
from pathlib import Path
import plistlib
import shutil
import subprocess
import zipfile


def stage(source, wheel, app, platform, arch="arm64", identity=None, certifi=None):
    source, wheel, app = Path(source).resolve(), Path(wheel).resolve(), Path(app).resolve()
    if app.suffix != ".app" or app.parent == app:
        raise ValueError("The runtime destination must be an app bundle")
    slice_name = "ios-arm64" if platform == "device" else "ios-arm64_x86_64-simulator"
    selected = source / slice_name
    shared = source / "lib/python3.13"
    native = selected / f"lib-{arch}/python3.13"
    framework = selected / "Python.framework"
    if certifi is None:
        raise ValueError("The downloader requires a pinned CA certificate bundle")
    for required in [shared / "os.py", shared / "encodings/__init__.py", native / "lib-dynload", framework / "Python", wheel, Path(certifi)]:
        if not required.exists():
            raise ValueError(f"Missing bundled Python input: {required}")
    if not list((native / "lib-dynload").glob("_ssl*.so")):
        raise ValueError("The bundled Python must include the native SSL module")
    for target in [app / "python", app / "Frameworks"]:
        if target.is_symlink() or not target.resolve().is_relative_to(app):
            raise ValueError("Runtime destination escapes the app bundle")
    runtime = app / "python"
    if runtime.exists():
        shutil.rmtree(runtime)
    stdlib = runtime / "lib/python3.13"
    shutil.copytree(shared, stdlib, ignore=shutil.ignore_patterns("__pycache__", "test", "tests"))
    shutil.copytree(native, stdlib, dirs_exist_ok=True, ignore=shutil.ignore_patterns("__pycache__"))
    frameworks = app / "Frameworks"
    frameworks.mkdir(parents=True, exist_ok=True)
    destination = frameworks / "Python.framework"
    if destination.exists():
        shutil.rmtree(destination)
    shutil.copytree(framework, destination)
    for name in ["Headers", "Modules"]:
        directory = destination / name
        if directory.exists():
            shutil.rmtree(directory)
    packages = stdlib / "site-packages"
    packages.mkdir(exist_ok=True)
    for package in [wheel, certifi]:
        with zipfile.ZipFile(package) as archive:
            for member in archive.infolist():
                if not (packages / member.filename).resolve().is_relative_to(packages.resolve()):
                    raise ValueError("Wheel member escapes site-packages")
            archive.extractall(packages)
    if not (packages / "certifi/cacert.pem").is_file():
        raise ValueError("The CA certificate bundle was not packaged")
    shutil.copyfile(Path(__file__).with_name("tdrive_mediadl.py"), packages / "tdrive_mediadl.py")
    for binary in sorted((stdlib / "lib-dynload").glob("*.so")):
        module = binary.name.split(".")[0]
        folder = frameworks / f"{module}.framework"
        folder.mkdir(exist_ok=True)
        executable = folder / module
        shutil.move(str(binary), executable)
        executable.chmod(0o755)
        info = {
            "CFBundleExecutable": module, "CFBundleIdentifier": "com.jeetraj.tdrive.python." + module.replace("_", "-"),
            "CFBundleName": module, "CFBundlePackageType": "FMWK", "CFBundleVersion": "3.13",
            "CFBundleShortVersionString": "3.13", "MinimumOSVersion": "16.0",
        }
        (folder / "Info.plist").write_bytes(plistlib.dumps(info))
        marker = binary.with_suffix(".fwork")
        marker.write_text(executable.relative_to(app).as_posix() + "\n")
        executable.with_suffix(".origin").write_text(marker.relative_to(app).as_posix() + "\n")
    if identity:
        for item in sorted(frameworks.glob("*.framework")):
            subprocess.run(["codesign", "--force", "--sign", identity, "--timestamp=none", str(item)], check=True)
    return stdlib


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--source", required=True)
    parser.add_argument("--wheel", required=True)
    parser.add_argument("--certifi", required=True)
    parser.add_argument("--app", required=True)
    parser.add_argument("--platform", choices=["device", "simulator"], required=True)
    parser.add_argument("--arch", default="arm64")
    parser.add_argument("--sign", default=None)
    args = parser.parse_args()
    stage(args.source, args.wheel, args.app, args.platform, args.arch, args.sign, args.certifi)
