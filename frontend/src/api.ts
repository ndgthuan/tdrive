// Public frontend boundary over generated Wails bindings.
// Domain implementations stay internal; UI modules import only from this file.
export * from "./api/drives";
export * from "./api/drive_members";
export * from "./api/files";
export * from "./api/resumable_uploads";
export * from "./api/resumable_downloads";
export * from "./api/gallery";
export * from "./api/media";
export * from "./api/renditions";
export * from "./api/mount";
export * from "./api/operation";
export * from "./api/runtime";
export { BackendInvocationError } from "./api/gateway";
export * from "./api/session";
export * from "./api/updates";
export * from "./api/channel_sources";
export * from "./api/biometric";

export type {
    DownloadResult,
    OperationError,
    OperationErrorCode,
    OperationResult,
    PreviewResult,
    UploadResult,
} from "./types";
