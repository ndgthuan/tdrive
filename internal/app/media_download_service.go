package app

import (
	"context"
	"errors"
	"sync"
	"time"

	"TDrive/backend/datadir"
	"TDrive/backend/mediadl"
	"TDrive/backend/projection"
	readservice "TDrive/backend/services/read"
)

// Event names for the download dialog. Progress belongs to a job; setup covers
// the yt-dlp and ffmpeg installs that run before any job can start.
const (
	MEDIA_DL_PROGRESS_EVENT = "media_dl:progress"
	MEDIA_DL_SETUP_EVENT    = "media_dl:setup"
	// MEDIA_DL_DEFAULT_FOLDER is the My Drive folder finished downloads land in.
	MEDIA_DL_DEFAULT_FOLDER = "Downloads"
	media_dl_update_timeout = 5 * time.Minute
)

// MediaSetupEvent reports one install step. Stage is "downloading", "verifying",
// "extracting", "done" or "error".
type MediaSetupEvent struct {
	Tool    string `json:"tool"`
	Stage   string `json:"stage"`
	Percent int    `json:"percent"`
	Error   string `json:"error,omitempty"`
}

// MediaDownloadService is the Wails face of the link downloader. It holds the
// tool locator, the installer and the job manager. The data directory is read
// on first use, not at construction, so building the service never touches disk.
type MediaDownloadService struct {
	host serviceHost

	init_once sync.Once
	init_err  error
	tools     *mediadl.Tools
	installer *mediadl.Installer
	jobs      *mediadl.Manager

	// setup_mu lets one tool install run at a time.
	setup_mu sync.Mutex
}

func new_media_download_service(host serviceHost) *MediaDownloadService {
	return &MediaDownloadService{host: host}
}

// ready builds the locator, installer and manager once. The manager clears the
// temp root of any earlier run here, before the first job can exist.
func (s *MediaDownloadService) ready() error {
	s.init_once.Do(func() {
		data_dir, err := datadir.Dir()
		if err != nil {
			s.init_err = err
			return
		}
		s.tools = mediadl.NewTools(data_dir)
		s.installer = mediadl.NewInstaller(s.tools.Paths())
		s.jobs = mediadl.NewManager(s.tools, mediadl.NewExecRunner(), media_destination{host: s.host}, func(event mediadl.Event) {
			s.host.emit(MEDIA_DL_PROGRESS_EVENT, event)
		})
	})
	return s.init_err
}

// Status reports whether yt-dlp and ffmpeg are present, and where they come from.
func (s *MediaDownloadService) Status() (mediadl.Status, error) {
	if err := s.ready(); err != nil {
		return mediadl.Status{}, err
	}
	return s.tools.Status(), nil
}

// Probe reads a link's title, duration and quality list without downloading.
func (s *MediaDownloadService) Probe(ctx context.Context, url string) (mediadl.ProbeResult, error) {
	if err := s.ready(); err != nil {
		return mediadl.ProbeResult{}, err
	}
	return s.jobs.Probe(ctx, url)
}

// Start begins a download into the drive's Downloads folder, or into
// target_folder_id when given. The drive is captured here, so a later drive
// switch cannot redirect the job. drive_id, when set, must match that drive.
func (s *MediaDownloadService) Start(ctx context.Context, request mediadl.StartRequest) (string, error) {
	if err := s.ready(); err != nil {
		return "", err
	}
	if err := s.host.WaitForBackendReady(ctx); err != nil {
		return "", err
	}
	channel_id := s.host.activeChannelID()
	if channel_id == 0 {
		return "", errBackendUnavailable
	}
	if request.DriveID != 0 && request.DriveID != channel_id {
		return "", errors.New("the active drive changed; reopen the download dialog")
	}
	return s.jobs.Start(s.host.appContext(), channel_id, request)
}

// Cancel stops a running download and removes its temp files.
func (s *MediaDownloadService) Cancel(job_id string) error {
	if err := s.ready(); err != nil {
		return err
	}
	return s.jobs.Cancel(job_id)
}

// ListJobs returns the current and recent downloads, newest first.
func (s *MediaDownloadService) ListJobs() ([]mediadl.JobView, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	return s.jobs.ListJobs(), nil
}

// InstallTool downloads yt-dlp or ffmpeg from its release page into the
// app-managed bin folder after verifying the published SHA-256.
func (s *MediaDownloadService) InstallTool(ctx context.Context, tool string) error {
	if err := s.ready(); err != nil {
		return err
	}
	s.setup_mu.Lock()
	defer s.setup_mu.Unlock()
	progress := func(stage string, percent int) {
		s.host.emit(MEDIA_DL_SETUP_EVENT, MediaSetupEvent{Tool: tool, Stage: stage, Percent: percent})
	}
	if err := s.installer.Install(ctx, tool, progress); err != nil {
		s.host.emit(MEDIA_DL_SETUP_EVENT, MediaSetupEvent{Tool: tool, Stage: "error", Error: err.Error()})
		return err
	}
	return nil
}

// UpdateYtDlp runs yt-dlp's own self-update on the copy the app will use.
func (s *MediaDownloadService) UpdateYtDlp(ctx context.Context) (string, error) {
	if err := s.ready(); err != nil {
		return "", err
	}
	s.setup_mu.Lock()
	defer s.setup_mu.Unlock()
	bin, err := s.tools.Locate(mediadl.TOOL_YTDLP)
	if err != nil {
		return "", errors.New("yt-dlp is not installed")
	}
	update_ctx, cancel := context.WithTimeout(ctx, media_dl_update_timeout)
	defer cancel()
	return mediadl.UpdateYtDlp(update_ctx, mediadl.NewExecRunner(), bin)
}

// SetToolPath points the downloader at a tool the user already has. An empty
// path clears the choice and falls back to PATH or the managed copy.
func (s *MediaDownloadService) SetToolPath(tool string, path string) error {
	if err := s.ready(); err != nil {
		return err
	}
	return s.tools.SetOverride(tool, path)
}

// media_destination adapts the drive services to the downloader's upload port.
// It adds no write path of its own: uploads go through the same file service
// as every other upload, and the folder is created through the folder service.
type media_destination struct {
	host serviceHost
}

// ResolveFolder returns folder_id when one was given. Otherwise it finds or
// creates the Downloads folder in the drive's root.
func (d media_destination) ResolveFolder(ctx context.Context, channel_id int64, folder_id string) (string, error) {
	if folder_id != "" {
		return folder_id, nil
	}
	engine := d.host.coreEngine()
	if engine == nil {
		return "", errBackendUnavailable
	}
	reads, err := engineReadService(engine)
	if err != nil {
		return "", err
	}
	folders := engine.FolderService()
	if folders == nil {
		return "", errBackendUnavailable
	}
	if id, found, err := find_root_folder(ctx, reads, channel_id, MEDIA_DL_DEFAULT_FOLDER); err != nil || found {
		return id, err
	}
	created, err := folders.CreateContext(ctx, channel_id, MEDIA_DL_DEFAULT_FOLDER, projection.RootParent)
	if err != nil {
		// A concurrent download may have created it first; look once more.
		if id, found, lookup_err := find_root_folder(ctx, reads, channel_id, MEDIA_DL_DEFAULT_FOLDER); lookup_err == nil && found {
			return id, nil
		}
		return "", err
	}
	return created.ID, nil
}

// find_root_folder looks for a folder named name directly under the drive root.
func find_root_folder(ctx context.Context, reads *readservice.Service, channel_id int64, name string) (string, bool, error) {
	listing, err := reads.FolderContentsContext(ctx, channel_id, projection.RootParent)
	if err != nil {
		return "", false, err
	}
	for _, folder := range listing.Folders {
		if folder.Name == name {
			return folder.ID, true, nil
		}
	}
	return "", false, nil
}

// UploadFile sends one finished file through the normal upload path.
func (d media_destination) UploadFile(ctx context.Context, channel_id int64, file_path string, folder_id string) error {
	svc, err := engineFileService(d.host.coreEngine())
	if err != nil {
		return err
	}
	_, failures, err := svc.UploadWithFailures(ctx, channel_id, []string{file_path}, []string{folder_id}, false)
	if err != nil {
		return err
	}
	if len(failures) > 0 {
		return errors.New(failures[0].Message)
	}
	return nil
}
