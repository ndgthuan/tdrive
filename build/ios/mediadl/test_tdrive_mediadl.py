import importlib.util
import sys
import tempfile
import types
import unittest
from pathlib import Path


SCRIPT = Path(__file__).with_name("tdrive_mediadl.py")
SPEC = importlib.util.spec_from_file_location("tdrive_mediadl_under_test", SCRIPT)
tdrive_mediadl = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(tdrive_mediadl)


class TDriveMediaDlNativeMergeTests(unittest.TestCase):
    def test_yt_dlp_merger_uses_host_callback_for_separate_streams(self):
        yt_dlp = types.ModuleType("yt_dlp")
        yt_dlp.__path__ = []
        postprocessor = types.ModuleType("yt_dlp.postprocessor")
        postprocessor.__path__ = []
        ffmpeg = types.ModuleType("yt_dlp.postprocessor.ffmpeg")
        utils = types.ModuleType("yt_dlp.utils")

        class FFmpegPostProcessorError(Exception):
            pass

        class FFmpegMergerPP:
            def __init__(self, downloader):
                self._downloader = downloader

            @property
            def available(self):
                return False

            def run(self, info):
                raise AssertionError("the default ffmpeg merger should not run")

            def to_screen(self, message):
                self._downloader.to_screen(message)

        ffmpeg.FFmpegMergerPP = FFmpegMergerPP
        ffmpeg.FFmpegPostProcessorError = FFmpegPostProcessorError
        utils.prepend_extension = lambda filename, ext: str(Path(filename).with_suffix(f".{ext}{Path(filename).suffix}"))
        yt_dlp.postprocessor = postprocessor
        postprocessor.ffmpeg = ffmpeg

        modules = {
            "yt_dlp": yt_dlp,
            "yt_dlp.postprocessor": postprocessor,
            "yt_dlp.postprocessor.ffmpeg": ffmpeg,
            "yt_dlp.utils": utils,
        }
        previous = {name: sys.modules.get(name) for name in modules}
        sys.modules.update(modules)
        try:
            with tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                video = root / "video.mp4"
                audio = root / "audio.m4a"
                output = root / "merged.mp4"
                video.write_bytes(b"video stream")
                audio.write_bytes(b"audio stream")
                calls = []

                class Downloader:
                    params = {}

                    def to_screen(self, message):
                        pass

                def main(_argv):
                    merger = FFmpegMergerPP(Downloader())
                    self.assertTrue(merger.available)
                    info = {
                        "filepath": str(output),
                        "requested_formats": [
                            {"filepath": str(video), "vcodec": "avc1", "acodec": "none"},
                            {"filepath": str(audio), "vcodec": "none", "acodec": "mp4a"},
                        ],
                        "__files_to_merge": [str(video), str(audio)],
                    }
                    merger.run(info)
                    self.assertEqual(output.read_bytes(), b"muxed video and audio")

                yt_dlp.main = main

                def native_merge(request_id, video_path, audio_path, output_path):
                    calls.append((request_id, video_path, audio_path))
                    Path(output_path).write_bytes(b"muxed video and audio")

                exit_code = tdrive_mediadl.run("request-42", [], lambda *_: None, native_merge)
                self.assertEqual(exit_code, 0)
                self.assertEqual(calls, [("request-42", str(video), str(audio))])
        finally:
            tdrive_mediadl._native_merger_installed = False
            tdrive_mediadl._native_merge_context.callback = None
            for name, value in previous.items():
                if value is None:
                    sys.modules.pop(name, None)
                else:
                    sys.modules[name] = value


if __name__ == "__main__":
    unittest.main()
