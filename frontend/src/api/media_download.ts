// Typed boundary for the link downloader. The dialog imports these names; the
// Go side is backend/mediadl and internal/app/media_download_service.go.
import { Events } from '@wailsio/runtime';
import {
    Cancel as rawCancel,
    CreateDestinationFolder as rawCreateDestinationFolder,
    ReportMediaDownloadLine as rawReportLine,
    ResolveMediaDownloadHost as rawResolveHost,
    InstallTool as rawInstallTool,
    ListDestinationFolders as rawListDestinationFolders,
    ListJobs as rawListJobs,
    Probe as rawProbe,
    SetToolPath as rawSetToolPath,
    Start as rawStart,
    Status as rawStatus,
    UpdateYtDlp as rawUpdateYtDlp,
} from '../../bindings/TDrive/internal/app/mediadownloadservice';
import type {
    JobView,
    ProbeResult,
    StartRequest,
    Status,
} from '../../bindings/TDrive/backend/mediadl/models';
import type { DestinationFolder, DestinationListing } from '../../bindings/TDrive/internal/app/models';
import { invokeBackend, noopRuntimeUnsubscribe, type RuntimeUnsubscribe } from './gateway';

export type { JobView, ProbeResult, ProbeFormat, Preset, StartRequest, Status, ToolStatus } from '../../bindings/TDrive/backend/mediadl/models';
export type { DestinationFolder, DestinationListing } from '../../bindings/TDrive/internal/app/models';

export const MEDIA_DL_PROGRESS_EVENT = 'media_dl:progress';
export const MEDIA_DL_SETUP_EVENT = 'media_dl:setup';
/** Carries one request from Go to the web view, which relays it to the phone's engine. */
export const MEDIA_DL_HOST_EVENT = 'media_dl:host';

/** One call the phone's native engine must answer. A cancel has no reply. */
export interface media_host_request {
    request_id: string;
    op: 'status' | 'install' | 'update' | 'run' | 'cancel';
    args?: string[];
}

/** One job update. Mirrors mediadl.Event; percent is -1 when unknown. */
export interface media_progress_event {
    job_id: string;
    stage: string;
    percent: number;
    speed: number;
    eta: number;
    title: string;
    error?: string;
    file_name?: string;
    folder_id?: string;
    drive_id?: number;
}

/** One install step. Mirrors MediaSetupEvent. */
export interface media_setup_event {
    tool: string;
    stage: string;
    percent: number;
    error?: string;
}

/** Reads whether yt-dlp and ffmpeg are available and where they come from. */
export function read_tool_status(): Promise<Status> {
    return invokeBackend(rawStatus);
}

/** Reads a link's title, uploader, duration, thumbnail and quality list. */
export function probe_link(url: string): Promise<ProbeResult> {
    return invokeBackend(rawProbe, url);
}

/** Starts a download to the drive. Returns the job id. */
export function start_download(request: StartRequest): Promise<string> {
    return invokeBackend(rawStart, request);
}

/** Subfolders of one folder in a drive. An empty parent_id is the drive's root. */
export function list_destination_folders(drive_id: number, parent_id: string): Promise<DestinationListing> {
    return invokeBackend(rawListDestinationFolders, drive_id, parent_id);
}

/** Creates a folder under parent_id in the drive and returns it. */
export function create_destination_folder(drive_id: number, parent_id: string, name: string): Promise<DestinationFolder> {
    return invokeBackend(rawCreateDestinationFolder, drive_id, parent_id, name);
}

/** Cancels a running job. */
export function cancel_download(job_id: string): Promise<void> {
    return invokeBackend(rawCancel, job_id);
}

/** Current and recent jobs, newest first. */
export async function list_downloads(): Promise<JobView[]> {
    const jobs = await invokeBackend(rawListJobs);
    return jobs ?? [];
}

/** Downloads yt-dlp or ffmpeg into TDrive's managed folder after checksum verification. */
export function install_tool(tool: 'yt-dlp' | 'ffmpeg'): Promise<void> {
    return invokeBackend(rawInstallTool, tool);
}

/** Runs yt-dlp's self-update. Resolves with yt-dlp's closing message. */
export function update_ytdlp(): Promise<string> {
    return invokeBackend(rawUpdateYtDlp);
}

/** Uses a tool the user already has. An empty path clears the choice. */
export function set_tool_path(tool: 'yt-dlp' | 'ffmpeg', path: string): Promise<void> {
    return invokeBackend(rawSetToolPath, tool, path);
}

/** Answers one native-engine request. failure, when set, is the message the user reads. */
export function reply_media_host(request_id: string, result: string, failure: string): Promise<void> {
    return invokeBackend(rawResolveHost, request_id, result, failure);
}

/** Passes one line of native output to the request that is still running. */
export function report_media_host_line(request_id: string, line: string): Promise<void> {
    return invokeBackend(rawReportLine, request_id, line);
}

/** Listens for native-engine requests. The returned function stops listening. */
export function subscribe_media_host(callback: (request: media_host_request) => void): RuntimeUnsubscribe {
    return subscribe<media_host_request>(MEDIA_DL_HOST_EVENT, callback);
}

function subscribe<T>(event_name: string, callback: (payload: T) => void): RuntimeUnsubscribe {
    let unsubscribe: (() => void) | void;
    try {
        unsubscribe = Events.On(event_name, (event) => {
            // Go emits one payload per event, wrapped in an array of its arguments.
            const data = Array.isArray(event.data) ? event.data : [event.data];
            if (data[0] !== undefined && data[0] !== null) callback(data[0] as T);
        });
    } catch {
        return noopRuntimeUnsubscribe;
    }
    if (typeof unsubscribe !== 'function') return noopRuntimeUnsubscribe;
    let active = true;
    return () => {
        if (!active) return;
        active = false;
        unsubscribe();
    };
}

/** Listens for job updates. The returned function stops listening. */
export function subscribe_media_progress(callback: (event: media_progress_event) => void): RuntimeUnsubscribe {
    return subscribe<media_progress_event>(MEDIA_DL_PROGRESS_EVENT, callback);
}

/** Listens for install steps. The returned function stops listening. */
export function subscribe_media_setup(callback: (event: media_setup_event) => void): RuntimeUnsubscribe {
    return subscribe<media_setup_event>(MEDIA_DL_SETUP_EVENT, callback);
}
