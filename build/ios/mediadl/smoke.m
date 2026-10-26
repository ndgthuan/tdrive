#define TDRIVE_REQUIRE_PYTHON 1
#import "../TDriveMediaDl.inc"
#import <CommonCrypto/CommonDigest.h>

static PyObject *SmokeLine(PyObject *self, PyObject *args) {
    const char *request, *line;
    if (!PyArg_ParseTuple(args, "ss", &request, &line)) return NULL;
    fprintf(stderr, "%s\n", line);
    Py_RETURN_NONE;
}
static PyMethodDef SmokeLineMethod = { "emit_line", SmokeLine, METH_VARARGS, NULL };

int main(int argc, char **argv) {
    @autoreleasepool {
        if (argc != 3 || !TDriveMediaDlPythonStarted()) return 1;
        NSString *json = TDriveMediaDlStatusJSON();
        NSDictionary *status = json ? [NSJSONSerialization JSONObjectWithData:[json dataUsingEncoding:NSUTF8StringEncoding] options:0 error:nil] : nil;
        if (![status[@"installed"] boolValue]) {
            fprintf(stderr, "Bundled Python/SSL/yt-dlp readiness failed\n");
            return 2;
        }
        NSString *output = [NSTemporaryDirectory() stringByAppendingPathComponent:@"tdrive-downloader-smoke.mp4"];
        NSArray *arguments = @[@"--ignore-config", @"--no-playlist", @"--quiet", @"--output", output, @"--", [NSString stringWithUTF8String:argv[1]]];
        PyGILState_STATE gil = PyGILState_Ensure();
        PyObject *module = PyImport_ImportModule("tdrive_mediadl");
        PyObject *args = PyList_New(0);
        for (NSString *argument in arguments) {
            PyObject *value = PyUnicode_FromString(argument.UTF8String);
            PyList_Append(args, value); Py_DECREF(value);
        }
        PyObject *callback = PyCFunction_New(&SmokeLineMethod, NULL);
        PyObject *result = module ? PyObject_CallMethod(module, "run", "sOO", "smoke", args, callback) : NULL;
        long code = result && PyLong_Check(result) ? PyLong_AsLong(result) : -1;
        if (PyErr_Occurred()) PyErr_Print();
        Py_XDECREF(result); Py_XDECREF(module); Py_DECREF(args); Py_DECREF(callback);
        PyGILState_Release(gil);
        if (code != 0) { fprintf(stderr, "yt-dlp download failed (%ld)\n", code); return 3; }
        NSData *data = [NSData dataWithContentsOfFile:output];
        unsigned char digest[CC_SHA256_DIGEST_LENGTH];
        CC_SHA256(data.bytes, (CC_LONG)data.length, digest);
        NSMutableString *hash = [NSMutableString string];
        for (NSUInteger index = 0; index < sizeof(digest); index++) [hash appendFormat:@"%02x", digest[index]];
        if (![hash isEqualToString:[NSString stringWithUTF8String:argv[2]]]) return 4;

        // Re-mux H.264 and AAC through the same AVFoundation path the app uses
        // for separate YouTube DASH streams. The fixture contains both tracks;
        // passing its URL twice supplies one track to each side of the muxer.
        TDriveMediaDlBridge *bridge = [[TDriveMediaDlBridge alloc] init];
        TDriveMediaDlActive = bridge;
        NSString *mergedOutput = [NSTemporaryDirectory() stringByAppendingPathComponent:@"tdrive-downloader-smoke-merged.mp4"];
        NSString *mergeError = nil;
        if (![bridge mergeRequestID:@"smoke-merge" videoPath:output audioPath:output outputPath:mergedOutput error:&mergeError]) {
            fprintf(stderr, "AVFoundation native mux failed: %s\n", mergeError.UTF8String ?: "unknown error");
            return 5;
        }
        AVURLAsset *mergedAsset = [AVURLAsset URLAssetWithURL:[NSURL fileURLWithPath:mergedOutput] options:nil];
        if ([[mergedAsset tracksWithMediaType:AVMediaTypeVideo] count] == 0 ||
            [[mergedAsset tracksWithMediaType:AVMediaTypeAudio] count] == 0) {
            fprintf(stderr, "Native mux output is missing an audio or video track\n");
            return 6;
        }
        [[NSFileManager defaultManager] removeItemAtPath:output error:nil];
        [[NSFileManager defaultManager] removeItemAtPath:mergedOutput error:nil];
        printf("TDRIVE_MEDIADL_SMOKE_OK %s\n", json.UTF8String);
        return 0;
    }
}
