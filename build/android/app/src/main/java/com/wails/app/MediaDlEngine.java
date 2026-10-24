package com.wails.app;

import android.content.Context;
import android.content.SharedPreferences;
import android.util.Log;
import com.yausername.ffmpeg.FFmpeg;
import com.yausername.youtubedl_android.YoutubeDL;
import com.yausername.youtubedl_android.YoutubeDLRequest;
import com.yausername.youtubedl_android.YoutubeDLResponse;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import org.json.JSONArray;
import org.json.JSONObject;

/**
 * yt-dlp on the phone, through youtubedl-android (com.yausername), pinned to
 * 0.18.1 in build.gradle. The Go side (backend/mediadl) decides every argument;
 * this class only turns the argv into a YoutubeDLRequest and runs it.
 *
 * Requests arrive from WailsJSBridge through the mediaDl* methods. Each answer
 * goes back through the callback the bridge was given, and each output line goes
 * to the web view as window.tdriveMediaDlLine(requestId, line), which relays it to Go.
 *
 * Unverified: this file has not been compiled against the library on this host.
 */
final class MediaDlEngine {
    private static final String TAG = "MediaDlEngine";
    private static final String PREFS = "tdrive_media_dl";
    private static final String READY_KEY = "engine_ready_v1";
    private static final ExecutorService worker = Executors.newCachedThreadPool();

    /** Receives one output line for a request. */
    interface LineSink {
        void line(String request_id, String text);
    }

    /** Receives the one answer for a request. A non-empty failure is shown to the user. */
    interface Done {
        void finish(String result, String failure);
    }

    private final Context app_context;
    private final LineSink sink;
    private boolean ffmpeg_ready;

    MediaDlEngine(Context app_context, LineSink sink) {
        this.app_context = app_context;
        this.sink = sink;
    }

    private SharedPreferences prefs() {
        return app_context.getSharedPreferences(PREFS, Context.MODE_PRIVATE);
    }

    /** Extracts the bundled Python and yt-dlp on first use. Later calls are cheap. */
    private synchronized void ensure_ready() throws Exception {
        YoutubeDL.getInstance().init(app_context);
        try {
            FFmpeg.getInstance().init(app_context);
            ffmpeg_ready = true;
        } catch (Exception e) {
            // yt-dlp still works for single-file formats without ffmpeg.
            Log.w(TAG, "ffmpeg is not available", e);
            ffmpeg_ready = false;
        }
    }

    /** Reports whether the engine has been set up and what version it runs. */
    void status(Done done) {
        worker.execute(() -> {
            try {
                boolean installed = prefs().getBoolean(READY_KEY, false);
                JSONObject answer = new JSONObject();
                answer.put("installed", installed);
                if (installed) {
                    ensure_ready();
                    answer.put("version", YoutubeDL.getInstance().version(app_context));
                    answer.put("ffmpeg", ffmpeg_ready);
                } else {
                    answer.put("version", "");
                    answer.put("ffmpeg", false);
                }
                done.finish(answer.toString(), "");
            } catch (Exception e) {
                done.finish("", message_of(e, "The video downloader could not be read."));
            }
        });
    }

    /** First-time setup. Reports "stage|percent" lines the Go side turns into progress. */
    void install(String request_id, Done done) {
        worker.execute(() -> {
            try {
                sink.line(request_id, "extracting|10");
                ensure_ready();
                prefs().edit().putBoolean(READY_KEY, true).apply();
                sink.line(request_id, "done|100");
                done.finish("", "");
            } catch (Exception e) {
                done.finish("", message_of(e, "The downloader could not be set up."));
            }
        });
    }

    /** Runs yt-dlp's own self-update. The result is a short closing message. */
    void update(Done done) {
        worker.execute(() -> {
            try {
                ensure_ready();
                YoutubeDL.UpdateStatus outcome = YoutubeDL.getInstance().updateYoutubeDL(app_context, YoutubeDL.UpdateChannel._STABLE);
                String version = YoutubeDL.getInstance().version(app_context);
                done.finish("yt-dlp " + version + " (" + outcome + ")", "");
            } catch (Exception e) {
                done.finish("", message_of(e, "yt-dlp could not update itself."));
            }
        });
    }

    /**
     * Runs one job. args_json is {"id":"...","args":[...]}, where args is the exact
     * yt-dlp argv the Go side built, URL last after "--".
     */
    void run(String args_json, Done done) {
        worker.execute(() -> {
            String request_id = "";
            try {
                JSONObject payload = new JSONObject(args_json);
                request_id = payload.getString("id");
                final String id = request_id;
                List<String> argv = to_list(payload.getJSONArray("args"));
                ensure_ready();
                YoutubeDLRequest request = build_request(argv);
                YoutubeDLResponse response = YoutubeDL.getInstance().execute(request, id, (progress, eta, line) -> {
                    sink.line(id, progress_line(progress, eta));
                    return kotlin.Unit.INSTANCE;
                });
                String out = response.getOut() == null ? "" : response.getOut();
                for (String line : out.split("\n")) {
                    if (!line.trim().isEmpty()) sink.line(id, line);
                }
                done.finish("", "");
            } catch (Exception e) {
                Log.w(TAG, "download failed", e);
                done.finish("", message_of(e, "The download failed."));
            }
        });
    }

    /** Stops a running job by the id it was started with. */
    void cancel(String request_id) {
        try {
            YoutubeDL.getInstance().destroyProcessById(request_id);
        } catch (Exception e) {
            Log.w(TAG, "cancel failed", e);
        }
    }

    /**
     * Builds the request from the argv. A flag followed by a value that does not
     * start with "-" is passed as a pair; everything after "--" is the URL.
     */
    static YoutubeDLRequest build_request(List<String> argv) {
        int separator = argv.indexOf("--");
        String url = separator >= 0 && separator + 1 < argv.size() ? argv.get(separator + 1) : "";
        YoutubeDLRequest request = new YoutubeDLRequest(url);
        int end = separator >= 0 ? separator : argv.size();
        for (int i = 0; i < end; i++) {
            String arg = argv.get(i);
            if (!arg.startsWith("-")) continue;
            if (i + 1 < end && !argv.get(i + 1).startsWith("-")) {
                request.addOption(arg, argv.get(i + 1));
                i++;
            } else {
                request.addOption(arg);
            }
        }
        return request;
    }

    /**
     * One progress line in the shape the Go side parses (see PROGRESS_TEMPLATE).
     * Percent is carried as downloaded bytes out of 100; speed is not reported.
     */
    static String progress_line(float progress, long eta_seconds) {
        long percent = Math.max(0, Math.min(100, Math.round(progress)));
        return "download:TDLP|" + percent + "|100|100|0|" + Math.max(0, eta_seconds);
    }

    private static List<String> to_list(JSONArray values) throws Exception {
        List<String> out = new ArrayList<>(values.length());
        for (int i = 0; i < values.length(); i++) out.add(values.getString(i));
        return out;
    }

    private static String message_of(Exception e, String fallback) {
        String message = e.getMessage();
        return message == null || message.trim().isEmpty() ? fallback : message;
    }
}
