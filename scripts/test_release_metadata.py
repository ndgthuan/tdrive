import json
from pathlib import Path
import tempfile
import unittest

from release_metadata import write_metadata


class ReleaseMetadataTests(unittest.TestCase):
    def test_inventory_pins_payload_sizes_and_can_be_regenerated(self):
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary)
            (directory / "TDrive-v3.1.1-windows-amd64.zip").write_bytes(b"payload")
            (directory / "checksums.txt").write_text("old manifest")
            for _ in range(2):
                write_metadata(directory, "owner/repo", "v3.1.1")
            metadata = json.loads((directory / "release.json").read_text())
            self.assertEqual(metadata["html_url"], "https://github.com/owner/repo/releases/tag/v3.1.1")
            self.assertEqual(metadata["assets"], [
                {"name": "TDrive-v3.1.1-windows-amd64.zip", "size": 7,
                 "browser_download_url": "https://github.com/owner/repo/releases/download/v3.1.1/TDrive-v3.1.1-windows-amd64.zip"},
                {"name": "checksums.txt", "size": 0,
                 "browser_download_url": "https://github.com/owner/repo/releases/download/v3.1.1/checksums.txt"},
                {"name": "checksums.txt.sig", "size": 0,
                 "browser_download_url": "https://github.com/owner/repo/releases/download/v3.1.1/checksums.txt.sig"},
            ])
            with self.assertRaises(ValueError):
                write_metadata(directory, "owner/repo", "v3.2.0-rc1")


if __name__ == "__main__":
    unittest.main()
