// Copyright (c) 2026 Nguyễn Dương Gia Thuận
//
// The link downloader's iOS engine. yt-dlp runs inside an embedded CPython that
// Taskfile stages into the app bundle as python/lib/python3.13 (see the
// mediadl:* tasks in build/ios/Taskfile.yml). The Go side reaches this file
// through window.tdriveMediaDl, which the web view relays.
//
// Not proven on a device or a Mac: the Python embedding, the Xcode header and
// link settings, and the bundle layout are all unverified. When the Python
// headers are not present at compile time, or the runtime is not staged in the
// bundle, status reports installed=false and the app shows "not installed"
// instead of failing.
//
// Native answers reach the page through __tdriveMediaDlSettle(promiseId, failure,
// result). Output lines reach it through tdriveMediaDlLine(requestId, line).

#import <UIKit/UIKit.h>
#import <WebKit/WebKit.h>
#import <objc/runtime.h>

#if __has_include(<Python.h>)
#import <Python.h>
#define TDRIVE_MEDIADL_PYTHON 1
#else
#define TDRIVE_MEDIADL_PYTHON 0
#endif

static NSString *const TDriveMediaDlHandlerName = @"tdriveMediaDl";
static NSString *const TDriveMediaDlPythonDirectory = @"python";
static NSString *const TDriveMediaDlStdlibFolder = @"lib/python3.13";
static char TDriveMediaDlBridgeKey;

@interface TDriveMediaDlBridge : NSObject <WKScriptMessageHandler>
@property (nonatomic, weak) WKWebView *webView;
- (void)settle:(NSString *)promiseID failure:(NSString *)failure result:(NSString *)result;
- (void)emitLine:(NSString *)requestID line:(NSString *)line;
@end

// The bridge that receives output lines from the Python callback. One web view
// hosts the app, so one active bridge is enough.
static __weak TDriveMediaDlBridge *TDriveMediaDlActive = nil;

static NSString *TDriveMediaDlPythonHome(void) {
    return [[[NSBundle mainBundle] bundlePath] stringByAppendingPathComponent:TDriveMediaDlPythonDirectory];
}

static NSString *TDriveMediaDlListJSON(NSArray *values) {
    NSData *data = [NSJSONSerialization dataWithJSONObject:values options:0 error:nil];
    return data ? [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding] : @"[]";
}

#if TDRIVE_MEDIADL_PYTHON

// Starts the embedded interpreter once. Returns NO when the runtime is not in
// the bundle or the interpreter will not start.
static BOOL TDriveMediaDlPythonStarted(void) {
    static BOOL started = NO;
    static dispatch_once_t once;
    dispatch_once(&once, ^{
        NSString *home = TDriveMediaDlPythonHome();
        NSString *stdlib = [home stringByAppendingPathComponent:TDriveMediaDlStdlibFolder];
        if (![[NSFileManager defaultManager] fileExistsAtPath:[stdlib stringByAppendingPathComponent:@"os.py"]]) return;
        PyConfig config;
        PyConfig_InitIsolatedConfig(&config);
        config.install_signal_handlers = 0;
        config.write_bytecode = 0;
        PyStatus status = PyConfig_SetBytesString(&config, &config.home, home.fileSystemRepresentation);
        if (!PyStatus_Exception(status)) status = Py_InitializeFromConfig(&config);
        PyConfig_Clear(&config);
        if (PyStatus_Exception(status)) return;
        started = YES;
        // Initialisation leaves the GIL held by this thread. Release it so the
        // worker threads can take it with PyGILState_Ensure.
        (void)PyEval_SaveThread();
    });
    return started;
}

// Called from Python for every output line: emit_line(request_id, line).
static PyObject *TDriveMediaDlEmitLine(PyObject *self, PyObject *args) {
    (void)self;
    const char *request_id = NULL;
    const char *line = NULL;
    if (!PyArg_ParseTuple(args, "ss", &request_id, &line)) return NULL;
    TDriveMediaDlBridge *bridge = TDriveMediaDlActive;
    NSString *requestID = [NSString stringWithUTF8String:request_id] ?: @"";
    NSString *text = [NSString stringWithUTF8String:line] ?: @"";
    [bridge emitLine:requestID line:text];
    Py_RETURN_NONE;
}

static PyMethodDef TDriveMediaDlLineMethod = { "emit_line", TDriveMediaDlEmitLine, METH_VARARGS, NULL };

// Calls tdrive_mediadl.<function>() and returns its result as JSON, or nil.
static NSString *TDriveMediaDlStatusJSON(void) {
    PyGILState_STATE gil = PyGILState_Ensure();
    NSString *json = nil;
    PyObject *module = PyImport_ImportModule("tdrive_mediadl");
    PyObject *result = module ? PyObject_CallMethod(module, "status", NULL) : NULL;
    PyObject *json_module = result ? PyImport_ImportModule("json") : NULL;
    PyObject *text = json_module ? PyObject_CallMethod(json_module, "dumps", "O", result) : NULL;
    if (text) json = [NSString stringWithUTF8String:PyUnicode_AsUTF8(text)];
    if (!text) PyErr_Clear();
    Py_XDECREF(text);
    Py_XDECREF(json_module);
    Py_XDECREF(result);
    Py_XDECREF(module);
    PyGILState_Release(gil);
    return json;
}

#endif

@implementation TDriveMediaDlBridge

- (void)settle:(NSString *)promiseID failure:(NSString *)failure result:(NSString *)result {
    NSString *list = TDriveMediaDlListJSON(@[promiseID ?: @"", failure ?: @"", result ?: @""]);
    NSString *script = [NSString stringWithFormat:@"window.__tdriveMediaDlSettle && window.__tdriveMediaDlSettle.apply(null, %@);", list];
    dispatch_async(dispatch_get_main_queue(), ^{ [self.webView evaluateJavaScript:script completionHandler:nil]; });
}

- (void)emitLine:(NSString *)requestID line:(NSString *)line {
    NSString *list = TDriveMediaDlListJSON(@[requestID ?: @"", line ?: @""]);
    NSString *script = [NSString stringWithFormat:@"window.tdriveMediaDlLine && window.tdriveMediaDlLine.apply(null, %@);", list];
    dispatch_async(dispatch_get_main_queue(), ^{ [self.webView evaluateJavaScript:script completionHandler:nil]; });
}

- (void)status:(NSString *)promiseID {
#if TDRIVE_MEDIADL_PYTHON
    dispatch_async(dispatch_get_global_queue(QOS_CLASS_UTILITY, 0), ^{
        NSString *json = nil;
        if (TDriveMediaDlPythonStarted()) json = TDriveMediaDlStatusJSON();
        if (json) {
            [self settle:promiseID failure:@"" result:json];
        } else {
            [self settle:promiseID failure:@"" result:@"{\"installed\":false,\"version\":\"\",\"ffmpeg\":false}"];
        }
    });
#else
    [self settle:promiseID failure:@"" result:@"{\"installed\":false,\"version\":\"\",\"ffmpeg\":false}"];
#endif
}

- (void)install:(NSString *)promiseID requestID:(NSString *)requestID {
    // The runtime is part of the bundle, so setup only confirms it starts and
    // reports the steps the Go side shows.
    [self emitLine:requestID line:@"extracting|10"];
#if TDRIVE_MEDIADL_PYTHON
    dispatch_async(dispatch_get_global_queue(QOS_CLASS_UTILITY, 0), ^{
        if (TDriveMediaDlPythonStarted()) {
            [self emitLine:requestID line:@"done|100"];
            [self settle:promiseID failure:@"" result:@""];
        } else {
            [self settle:promiseID failure:@"The downloader is not installed in this build." result:@""];
        }
    });
#else
    [self settle:promiseID failure:@"The downloader is not installed in this build." result:@""];
#endif
}

- (void)update:(NSString *)promiseID {
    // iOS cannot install packages at runtime, so yt-dlp moves forward with app
    // updates. The reply says so instead of pretending to update.
    [self status:promiseID];
}

- (void)run:(NSString *)promiseID requestID:(NSString *)requestID argsJSON:(NSString *)argsJSON {
#if TDRIVE_MEDIADL_PYTHON
    dispatch_async(dispatch_get_global_queue(QOS_CLASS_UTILITY, 0), ^{
        NSArray *argv = [NSJSONSerialization JSONObjectWithData:[argsJSON ?: @"[]" dataUsingEncoding:NSUTF8StringEncoding] options:0 error:nil];
        if (![argv isKindOfClass:[NSArray class]] || !TDriveMediaDlPythonStarted()) {
            [self settle:promiseID failure:@"The downloader is not installed in this build." result:@""];
            return;
        }
        PyGILState_STATE gil = PyGILState_Ensure();
        PyObject *module = PyImport_ImportModule("tdrive_mediadl");
        PyObject *args = PyList_New(0);
        for (id value in argv) {
            PyObject *item = PyUnicode_FromString([[value description] UTF8String]);
            if (item) { PyList_Append(args, item); Py_DECREF(item); }
        }
        PyObject *callback = PyCFunction_New(&TDriveMediaDlLineMethod, NULL);
        PyObject *code = (module && args && callback)
            ? PyObject_CallMethod(module, "run", "sOO", [requestID UTF8String], args, callback)
            : NULL;
        long exit_code = (code && PyLong_Check(code)) ? PyLong_AsLong(code) : -1;
        if (!code) PyErr_Clear();
        Py_XDECREF(code);
        Py_XDECREF(callback);
        Py_XDECREF(args);
        Py_XDECREF(module);
        PyGILState_Release(gil);
        if (exit_code == 0) {
            [self settle:promiseID failure:@"" result:@""];
        } else {
            [self settle:promiseID failure:@"The download failed." result:@""];
        }
    });
#else
    [self settle:promiseID failure:@"The downloader is not installed in this build." result:@""];
#endif
}

- (void)cancel:(NSString *)requestID {
#if TDRIVE_MEDIADL_PYTHON
    if (requestID.length == 0 || !TDriveMediaDlPythonStarted()) return;
    PyGILState_STATE gil = PyGILState_Ensure();
    PyObject *module = PyImport_ImportModule("tdrive_mediadl");
    PyObject *result = module ? PyObject_CallMethod(module, "cancel", "s", [requestID UTF8String]) : NULL;
    if (!result) PyErr_Clear();
    Py_XDECREF(result);
    Py_XDECREF(module);
    PyGILState_Release(gil);
#endif
}

- (void)userContentController:(WKUserContentController *)controller didReceiveScriptMessage:(WKScriptMessage *)message {
    (void)controller;
    NSURL *origin = message.frameInfo.request.URL;
    NSString *host = origin.host.lowercaseString ?: @"";
    BOOL trustedOrigin = [origin.scheme isEqualToString:@"wails"]
        || (([origin.scheme isEqualToString:@"http"] || [origin.scheme isEqualToString:@"https"])
            && ([host isEqualToString:@"localhost"] || [host isEqualToString:@"127.0.0.1"] || [host isEqualToString:@"::1"] || [host hasSuffix:@".localhost"]));
    if (!message.frameInfo.isMainFrame || !trustedOrigin) return;
    NSDictionary *body = [message.body isKindOfClass:[NSDictionary class]] ? message.body : nil;
    NSString *promiseID = [body[@"id"] isKindOfClass:[NSString class]] ? body[@"id"] : @"";
    NSString *operation = [body[@"operation"] isKindOfClass:[NSString class]] ? body[@"operation"] : @"";
    NSDictionary *arguments = [body[@"arguments"] isKindOfClass:[NSDictionary class]] ? body[@"arguments"] : @{};
    NSString *requestID = [arguments[@"id"] isKindOfClass:[NSString class]] ? arguments[@"id"] : @"";
    if ([operation isEqualToString:@"status"]) [self status:promiseID];
    else if ([operation isEqualToString:@"install"]) [self install:promiseID requestID:requestID];
    else if ([operation isEqualToString:@"update"]) [self update:promiseID];
    else if ([operation isEqualToString:@"run"]) [self run:promiseID requestID:requestID argsJSON:[arguments[@"argsJson"] isKindOfClass:[NSString class]] ? arguments[@"argsJson"] : @"[]"];
    else if ([operation isEqualToString:@"cancel"]) [self cancel:requestID];
    else [self settle:promiseID failure:@"Unsupported download operation" result:@""];
}

@end

// Installs the bridge on a web view. Call once, after the web view exists.
void TDriveInstallMediaDlBridge(WKWebView *webView, UIViewController *presenter) {
    (void)presenter;
    if (webView == nil || objc_getAssociatedObject(webView, &TDriveMediaDlBridgeKey) != nil) return;
    TDriveMediaDlBridge *bridge = [[TDriveMediaDlBridge alloc] init];
    bridge.webView = webView;
    TDriveMediaDlActive = bridge;
    objc_setAssociatedObject(webView, &TDriveMediaDlBridgeKey, bridge, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
    WKUserContentController *controller = webView.configuration.userContentController;
    [controller addScriptMessageHandler:bridge name:TDriveMediaDlHandlerName];
    NSString *source = @"(function(){if(window.tdriveMediaDl)return;const pending=new Map();let seq=0;window.__tdriveMediaDlSettle=function(id,failure,result){const p=pending.get(id);if(!p)return;pending.delete(id);if(failure)p.reject(Object.assign(new Error(failure),{code:'mediaDlFailed'}));else p.resolve(result)};const post=(operation,args)=>{const id=Date.now().toString(36)+'-'+(++seq).toString(36);return new Promise((resolve,reject)=>{pending.set(id,{resolve,reject});window.webkit.messageHandlers.tdriveMediaDl.postMessage({id,operation,arguments:args||{}})})};window.tdriveMediaDl=Object.freeze({status:()=>post('status'),install:(id)=>post('install',{id}),update:()=>post('update'),run:(id,argsJson)=>post('run',{id,argsJson}),cancel:(id)=>{window.webkit.messageHandlers.tdriveMediaDl.postMessage({id:'',operation:'cancel',arguments:{id}})}})})();";
    [controller addUserScript:[[WKUserScript alloc] initWithSource:source injectionTime:WKUserScriptInjectionTimeAtDocumentStart forMainFrameOnly:YES]];
    [webView evaluateJavaScript:source completionHandler:nil];
}
