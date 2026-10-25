import tempfile
from pathlib import Path
import unittest
import zipfile

from package_runtime import stage


class RuntimePackagingTests(unittest.TestCase):
    def test_shared_stdlib_native_modules_framework_and_wheel_are_packaged(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = root / "Python.xcframework"
            for file in ["lib/python3.13/os.py", "lib/python3.13/encodings/__init__.py",
                         "ios-arm64/lib-arm64/python3.13/lib-dynload/_ssl.cpython-313-iphoneos.so",
                         "ios-arm64/Python.framework/Python"]:
                path = source / file
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_bytes(b"fixture")
            wheel = root / "yt_dlp.whl"
            with zipfile.ZipFile(wheel, "w") as archive:
                archive.writestr("yt_dlp/version.py", "__version__ = 'test'")
            certifi = root / "certifi.whl"
            with zipfile.ZipFile(certifi, "w") as archive:
                archive.writestr("certifi/cacert.pem", "test certificate bundle")
            app = root / "TDrive.app"
            stdlib = stage(source, wheel, app, "device", certifi=certifi)
            self.assertTrue((stdlib / "os.py").is_file())
            self.assertTrue((stdlib / "encodings/__init__.py").is_file())
            self.assertTrue((stdlib / "site-packages/yt_dlp/version.py").is_file())
            self.assertTrue((stdlib / "site-packages/tdrive_mediadl.py").is_file())
            self.assertEqual((app / "Frameworks/Python.framework/Python").read_bytes(), b"fixture")
            marker = stdlib / "lib-dynload/_ssl.cpython-313-iphoneos.fwork"
            self.assertEqual(marker.read_text().strip(), "Frameworks/_ssl.framework/_ssl")
            self.assertEqual((app / "Frameworks/_ssl.framework/_ssl.origin").read_text().strip(),
                             "python/lib/python3.13/lib-dynload/_ssl.cpython-313-iphoneos.fwork")
            (source / "lib/python3.13/os.py").unlink()
            with self.assertRaisesRegex(ValueError, "Missing bundled Python input"):
                stage(source, wheel, app, "device", certifi=certifi)
            self.assertTrue((stdlib / "os.py").exists(), "invalid inputs must not erase the staged runtime")


if __name__ == "__main__":
    unittest.main()
