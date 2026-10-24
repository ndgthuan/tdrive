package app

import (
	"context"
	"errors"
	"sort"
	"strings"
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

// Start begins a download into the drive and folder the request names. DriveID 0
// is the active drive, and an empty TargetFolderID is that drive's root. The
// destination is checked before the job exists, so a refused start leaves nothing
// behind.
func (s *MediaDownloadService) Start(ctx context.Context, request mediadl.StartRequest) (string, error) {
	if err := s.ready(); err != nil {
		return "", err
	}
	if err := s.host.WaitForBackendReady(ctx); err != nil {
		return "", err
	}
	channel_id := request.DriveID
	if channel_id == 0 {
		channel_id = s.host.activeChannelID()
	}
	if channel_id == 0 {
		return "", errBackendUnavailable
	}
	return s.jobs.Start(s.host.appContext(), channel_id, request)
}

// DestinationFolder is one folder offered by the download destination picker.
type DestinationFolder struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// DestinationListing holds the subfolders of one folder, sorted by name.
type DestinationListing struct {
	Folders []DestinationFolder `json:"folders"`
}

// ListDestinationFolders lists the subfolders of parent_id in drive_id, an empty
// parent_id meaning the drive's root. The drive need not be the active one, so
// the picker can browse every drive the user has joined.
func (s *MediaDownloadService) ListDestinationFolders(ctx context.Context, drive_id int64, parent_id string) (DestinationListing, error) {
	if err := s.host.WaitForBackendReady(ctx); err != nil {
		return DestinationListing{}, err
	}
	dest := media_destination{host: s.host}
	reads, err := dest.reads(drive_id)
	if err != nil {
		return DestinationListing{}, err
	}
	if err := dest.require_folder(reads, drive_id, parent_id); err != nil {
		return DestinationListing{}, err
	}
	listing, err := reads.FolderContentsContext(ctx, drive_id, parent_id)
	if err != nil {
		return DestinationListing{}, err
	}
	folders := make([]DestinationFolder, 0, len(listing.Folders))
	for _, folder := range listing.Folders {
		folders = append(folders, DestinationFolder{ID: folder.ID, Name: folder.Name})
	}
	sort.SliceStable(folders, func(i, k int) bool {
		return strings.ToLower(folders[i].Name) < strings.ToLower(folders[k].Name)
	})
	return DestinationListing{Folders: folders}, nil
}

// CreateDestinationFolder creates a folder under parent_id in drive_id through
// the folder service the file list's New folder action also uses.
func (s *MediaDownloadService) CreateDestinationFolder(ctx context.Context, drive_id int64, parent_id string, name string) (DestinationFolder, error) {
	if err := s.host.WaitForBackendReady(ctx); err != nil {
		return DestinationFolder{}, err
	}
	dest := media_destination{host: s.host}
	reads, err := dest.reads(drive_id)
	if err != nil {
		return DestinationFolder{}, err
	}
	if err := dest.require_folder(reads, drive_id, parent_id); err != nil {
		return DestinationFolder{}, err
	}
	folders := s.host.coreEngine().FolderService()
	if folders == nil {
		return DestinationFolder{}, errBackendUnavailable
	}
	created, err := folders.CreateContext(ctx, drive_id, name, parent_id)
	if err != nil {
		return DestinationFolder{}, err
	}
	return DestinationFolder{ID: created.ID, Name: created.Name}, nil
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

// media_destination adapts the drive services to the downloader's destination
// port. It adds no write path of its own: uploads go through the same file
// service as every other upload, and folders through the folder service.
type media_destination struct {
	host serviceHost
}

// reads returns the read service after checking that drive_id is a drive the
// user has joined. A drive the user has left has no channel row.
func (d media_destination) reads(drive_id int64) (*readservice.Service, error) {
	reads, err := engineReadService(d.host.coreEngine())
	if err != nil {
		return nil, err
	}
	if _, err := projection.GetChannel(reads.DB, drive_id); err != nil {
		return nil, errors.New("that drive is not available; reopen the download dialog")
	}
	return reads, nil
}

// require_folder accepts the drive root, or a folder that still exists in the drive.
func (d media_destination) require_folder(reads *readservice.Service, drive_id int64, folder_id string) error {
	if folder_id == "" {
		return nil
	}
	if !projection.IsFolderID(folder_id) || !projection.FolderExists(reads.DB, drive_id, folder_id) {
		return errors.New("that folder no longer exists; choose another folder")
	}
	return nil
}

// CheckTarget runs before a download starts. It requires a joined drive, an
// existing folder, and the same peer resolution every upload performs first.
// That resolution is the membership gate the upload path relies on; the upload
// path has no separate role check to reuse.
func (d media_destination) CheckTarget(ctx context.Context, channel_id int64, folder_id string) error {
	reads, err := d.reads(channel_id)
	if err != nil {
		return err
	}
	if err := d.require_folder(reads, channel_id, folder_id); err != nil {
		return err
	}
	files, err := engineFileService(d.host.coreEngine())
	if err != nil {
		return err
	}
	if files.Peers == nil {
		return errBackendUnavailable
	}
	_, err = files.Peers.ResolvePeer(ctx, channel_id)
	return err
}

// UploadFile sends one finished file through the normal upload path. An empty
// folder_id uploads to the drive's root.
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
