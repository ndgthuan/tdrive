//go:build windows

package mountos

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var wnetGetConnection = windows.NewLazySystemDLL("mpr.dll").NewProc("WNetGetConnectionW")

const windowsMappingVerificationTimeout = 3 * time.Second

// windows_mount_label is the Explorer display name for the mapped drive. It is
// applied through the MountPoints2 _LabelFromReg value, so the capability path
// that net use mounts never appears in the label. Explorer still shows the
// UNC remote in the tooltip and in net use output.
const windows_mount_label = "Telegram Drive"

type windowsDependencies struct {
	runner            commandRunner
	systemPaths       func() (netExecutable string, explorerExecutable string, err error)
	logicalDrives     func() (uint32, error)
	remoteFor         func(string) (remote string, mapped bool, err error)
	isRemoteDrive     func(string) (bool, error)
	prepareWebClient  func(context.Context) error
	verificationDelay func(context.Context) error
	// set_mount_label and clear_mount_label default to no-ops so tests never
	// touch the real registry; newPlatformConnector wires the registry versions.
	set_mount_label   func(remote, label string) error
	clear_mount_label func(remote string) error
	// notify_drive defaults to a no-op; it asks Explorer to refresh the drive.
	notify_drive func(drive string)
}

type windowsConnector struct {
	mu                sync.Mutex
	owner             *ownerMarker
	runner            commandRunner
	systemPaths       func() (string, string, error)
	logicalDrives     func() (uint32, error)
	remoteFor         func(string) (string, bool, error)
	isRemoteDrive     func(string) (bool, error)
	prepareWebClient  func(context.Context) error
	verificationDelay func(context.Context) error
	set_mount_label   func(remote, label string) error
	clear_mount_label func(remote string) error
	notify_drive      func(drive string)
	serial            uint64
	active            map[string]windowsActive
}

type windowsActive struct {
	id       uint64
	remote   string
	endpoint string
}

func newPlatformConnector() Connector {
	return newWindowsConnector(windowsDependencies{
		set_mount_label:   windows_set_mount_label,
		clear_mount_label: windows_clear_mount_label,
		notify_drive:      windows_notify_drive_changed,
	})
}

func newWindowsConnector(dependencies windowsDependencies) *windowsConnector {
	runner := dependencies.runner
	if runner == nil {
		runner = execCommandRunner{}
	}
	systemPaths := dependencies.systemPaths
	if systemPaths == nil {
		systemPaths = windowsSystemPaths
	}
	logicalDrives := dependencies.logicalDrives
	if logicalDrives == nil {
		logicalDrives = windows.GetLogicalDrives
	}
	remoteFor := dependencies.remoteFor
	if remoteFor == nil {
		remoteFor = windowsRemoteFor
	}
	isRemoteDrive := dependencies.isRemoteDrive
	if isRemoteDrive == nil {
		isRemoteDrive = windowsIsRemoteDrive
	}
	prepareWebClient := dependencies.prepareWebClient
	if prepareWebClient == nil {
		prepareWebClient = ensureWindowsWebClient
	}
	verificationDelay := dependencies.verificationDelay
	if verificationDelay == nil {
		verificationDelay = func(ctx context.Context) error {
			return waitForPoll(ctx, 100*time.Millisecond)
		}
	}
	set_mount_label := dependencies.set_mount_label
	if set_mount_label == nil {
		set_mount_label = func(string, string) error { return nil }
	}
	clear_mount_label := dependencies.clear_mount_label
	if clear_mount_label == nil {
		clear_mount_label = func(string) error { return nil }
	}
	notify_drive := dependencies.notify_drive
	if notify_drive == nil {
		notify_drive = func(string) {}
	}
	return &windowsConnector{
		owner:             &ownerMarker{},
		runner:            runner,
		systemPaths:       systemPaths,
		logicalDrives:     logicalDrives,
		remoteFor:         remoteFor,
		isRemoteDrive:     isRemoteDrive,
		prepareWebClient:  prepareWebClient,
		verificationDelay: verificationDelay,
		set_mount_label:   set_mount_label,
		clear_mount_label: clear_mount_label,
		notify_drive:      notify_drive,
		active:            make(map[string]windowsActive),
	}
}

func (c *windowsConnector) Attach(parent context.Context, config Config) (Attachment, error) {
	ctx, cancel, err := boundedContext(parent, attachTimeout)
	if err != nil {
		return Attachment{}, err
	}
	defer cancel()
	validated, err := validateConfig(config)
	if err != nil {
		return Attachment{}, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	// validated.endpoint and the "remote" UNC strings returned by
	// c.remoteFor/windowsRemoteFor embed the loopback capability token and
	// must never be logged. Only validated.drive (e.g. "T:") is safe.
	drives, err := c.logicalDrives()
	if err != nil {
		return Attachment{}, ErrAttachFailed
	}
	if drives&(uint32(1)<<uint(validated.drive[0]-'A')) != 0 {
		slog.Warn("mountos: windows attach rejected, drive letter occupied", "drive", validated.drive)
		return Attachment{}, ErrDriveOccupied
	}
	if err := c.prepareWebClient(ctx); err != nil {
		if ctx.Err() != nil {
			return Attachment{}, ctx.Err()
		}
		slog.Warn("mountos: windows WebClient service unavailable", "drive", validated.drive, "error", err)
		return Attachment{}, ErrWindowsWebDAVUnavailable
	}
	netExecutable, _, err := c.systemPaths()
	if err != nil {
		return Attachment{}, ErrAttachFailed
	}
	// Explorer reads the label when it first enumerates the new drive, which is
	// during net use, so the keys are written from the endpoint before the
	// mapping exists. The same keys are written again once the real remote is
	// verified. Every failure path after this point clears them (deferred).
	// The mapping targets the root folder under the endpoint, not the endpoint
	// itself, so Explorer names the drive after WindowsRootFolder.
	mapped_endpoint := windowsRootEndpoint(validated.endpoint)
	expected, has_expected := windowsExpectedRemote(mapped_endpoint)
	labelled := false
	if has_expected {
		c.apply_label_best_effort(validated.drive, expected)
		defer func() {
			if !labelled {
				c.clear_label_variants_best_effort(validated.drive, validated.endpoint)
			}
		}()
	}
	slog.Debug("mountos: windows attaching", "drive", validated.drive, "mode", validated.mode)
	if err := c.runner.Run(ctx, windowsAttachPlan(netExecutable, validated.drive, mapped_endpoint)); err != nil {
		slog.Warn("mountos: windows net use failed", "drive", validated.drive, "error", err)
		c.rollbackUnknown(parent, netExecutable, validated.drive, validated.endpoint)
		if ctx.Err() != nil {
			return Attachment{}, ctx.Err()
		}
		return Attachment{}, ErrWindowsWebDAVUnavailable
	}
	verificationCtx, verificationCancel := context.WithTimeout(ctx, windowsMappingVerificationTimeout)
	remote, err := c.waitForVerifiedMapping(verificationCtx, validated.drive, validated.endpoint)
	verificationCancel()
	if err != nil {
		if ctx.Err() != nil {
			return Attachment{}, ctx.Err()
		}
		if errors.Is(err, ErrAttachmentChanged) {
			// A concurrent actor or provider anomaly rebound the drive. It is not
			// ours, so never attempt to remove it.
			slog.Warn("mountos: windows drive was rebound by another actor during attach", "drive", validated.drive)
			return Attachment{}, ErrAttachmentChanged
		}
		slog.Warn("mountos: windows attach verification failed", "drive", validated.drive, "error", err)
		if remote != "" && windowsRemoteMatchesEndpoint(remote, validated.endpoint) {
			c.rollback(parent, netExecutable, validated.drive, remote)
		} else {
			c.rollbackUnknown(parent, netExecutable, validated.drive, validated.endpoint)
		}
		return Attachment{}, ErrVerificationFailed
	}

	id := c.nextID()
	c.active[validated.drive] = windowsActive{id: id, remote: remote, endpoint: validated.endpoint}
	// The label is cosmetic: a failure leaves the mount usable, so it only logs.
	// The remote is never logged; the drive letter identifies the mapping.
	c.apply_label_best_effort(validated.drive, remote)
	if has_expected && remote != expected {
		// The verified remote is spelled differently from the endpoint-derived
		// one, so the pre-mapping keys are not used by Explorer. Remove them.
		c.clear_label_best_effort(validated.drive, expected)
	}
	c.notify_drive(validated.drive + `\`)
	labelled = true
	slog.Info("mountos: windows attached", "drive", validated.drive, "mode", validated.mode)
	return Attachment{
		owner:    c.owner,
		id:       id,
		kind:     KindWindows,
		location: validated.drive + `\`,
	}, nil
}

func (c *windowsConnector) waitForVerifiedMapping(ctx context.Context, drive, endpoint string) (string, error) {
	lastOwnedRemote := ""
	for {
		remote, mapped, err := c.remoteFor(drive)
		if err == nil && mapped && remote != "" {
			if !windowsRemoteMatchesEndpoint(remote, endpoint) {
				return "", ErrAttachmentChanged
			}
			lastOwnedRemote = remote
			remoteDrive, driveErr := c.isRemoteDrive(drive + `\`)
			if driveErr == nil && remoteDrive {
				return remote, nil
			}
		}
		if err := c.verificationDelay(ctx); err != nil {
			return lastOwnedRemote, ErrVerificationFailed
		}
	}
}

func (c *windowsConnector) Detach(parent context.Context, attachment Attachment) error {
	ctx, cancel, err := boundedContext(parent, detachTimeout)
	if err != nil {
		return err
	}
	defer cancel()

	c.mu.Lock()
	defer c.mu.Unlock()
	drive, active, ok := c.ownedDrive(attachment)
	if !ok {
		return ErrInvalidAttachment
	}
	current, mapped, err := c.remoteFor(drive)
	if err != nil {
		return ErrVerificationFailed
	}
	if !mapped {
		c.clear_label_variants_best_effort(drive, active.endpoint)
		delete(c.active, drive)
		return nil
	}
	if current != active.remote {
		return ErrAttachmentChanged
	}
	netExecutable, _, err := c.systemPaths()
	if err != nil {
		return ErrDetachFailed
	}
	if err := c.runner.Run(ctx, windowsDetachPlan(netExecutable, drive)); err != nil {
		slog.Warn("mountos: windows net use delete failed", "drive", drive, "error", err)
		return ErrDetachFailed
	}
	_, mapped, err = c.remoteFor(drive)
	if err != nil || mapped {
		return ErrVerificationFailed
	}
	c.clear_label_variants_best_effort(drive, active.endpoint)
	delete(c.active, drive)
	slog.Info("mountos: windows detached", "drive", drive)
	return nil
}

func (c *windowsConnector) Open(parent context.Context, attachment Attachment) error {
	ctx, cancel, err := boundedContext(parent, openTimeout)
	if err != nil {
		return err
	}
	defer cancel()

	c.mu.Lock()
	defer c.mu.Unlock()
	drive, active, ok := c.ownedDrive(attachment)
	if !ok {
		return ErrInvalidAttachment
	}
	current, mapped, err := c.remoteFor(drive)
	if err != nil {
		return ErrVerificationFailed
	}
	if !mapped || current != active.remote {
		return ErrAttachmentChanged
	}
	remoteDrive, err := c.isRemoteDrive(attachment.location)
	if err != nil {
		return ErrVerificationFailed
	}
	if !remoteDrive {
		return ErrAttachmentChanged
	}
	_, explorerExecutable, err := c.systemPaths()
	if err != nil {
		return ErrOpenFailed
	}
	if err := c.runner.Run(ctx, windowsOpenPlan(explorerExecutable, attachment.location)); err != nil {
		return ErrOpenFailed
	}
	return nil
}

func (c *windowsConnector) ownedDrive(attachment Attachment) (string, windowsActive, bool) {
	if attachment.owner != c.owner || attachment.kind != KindWindows || len(attachment.location) != 3 {
		return "", windowsActive{}, false
	}
	drive := attachment.location[:2]
	active, exists := c.active[drive]
	return drive, active, exists && attachment.id != 0 && active.id == attachment.id
}

// apply_label_best_effort writes the Explorer label for remote. The label is
// cosmetic, so failure is logged only. The remote is never logged.
func (c *windowsConnector) apply_label_best_effort(drive, remote string) {
	if err := c.set_mount_label(remote, windows_mount_label); err != nil {
		slog.Warn("mountos: windows drive label not applied", "drive", drive, "error", err)
	}
}

// clear_label_best_effort removes the label key for a mapping that is already
// gone. Failure is logged only, because the mapping itself is no longer present.
func (c *windowsConnector) clear_label_best_effort(drive, remote string) {
	if err := c.clear_mount_label(remote); err != nil {
		slog.Warn("mountos: windows drive label cleanup failed", "drive", drive, "error", err)
	}
}

// clear_label_variants_best_effort removes the label keys for every remote
// spelling this endpoint can be reported as: the root-folder mapping, and the
// bare endpoint from earlier builds. Each spelling is a per-mount key.
func (c *windowsConnector) clear_label_variants_best_effort(drive, endpoint string) {
	for _, remote := range windowsAcceptedRemotes(endpoint) {
		c.clear_label_best_effort(drive, remote)
	}
}

func (c *windowsConnector) nextID() uint64 {
	c.serial++
	if c.serial == 0 {
		c.serial++
	}
	return c.serial
}

func (c *windowsConnector) rollbackUnknown(parent context.Context, netExecutable, drive, endpoint string) {
	remote, mapped, err := c.remoteFor(drive)
	if err != nil || !mapped || !windowsRemoteMatchesEndpoint(remote, endpoint) {
		return
	}
	c.rollback(parent, netExecutable, drive, remote)
}

func (c *windowsConnector) rollback(parent context.Context, netExecutable, drive, expectedRemote string) {
	current, mapped, err := c.remoteFor(drive)
	if err != nil || !mapped || current != expectedRemote {
		return
	}
	var cleanupParent context.Context = context.Background()
	if parent != nil {
		cleanupParent = context.WithoutCancel(parent)
	}
	ctx, cancel := context.WithTimeout(cleanupParent, detachTimeout)
	defer cancel()
	_ = c.runner.Run(ctx, windowsDetachPlan(netExecutable, drive))
	_, _, _ = c.remoteFor(drive)
}

func windowsSystemPaths() (string, string, error) {
	systemDir, err := windows.GetSystemDirectory()
	if err != nil || !filepath.IsAbs(systemDir) {
		return "", "", ErrAttachFailed
	}
	windowsDir, err := windows.GetWindowsDirectory()
	if err != nil || !filepath.IsAbs(windowsDir) {
		return "", "", ErrAttachFailed
	}
	return filepath.Join(systemDir, "net.exe"), filepath.Join(windowsDir, "explorer.exe"), nil
}

func windowsRemoteFor(drive string) (string, bool, error) {
	localName, err := windows.UTF16PtrFromString(drive)
	if err != nil {
		return "", false, err
	}
	size := uint32(512)
	for attempts := 0; attempts < 2; attempts++ {
		buffer := make([]uint16, size)
		result, _, _ := wnetGetConnection.Call(
			uintptr(unsafe.Pointer(localName)),
			uintptr(unsafe.Pointer(&buffer[0])),
			uintptr(unsafe.Pointer(&size)),
		)
		status := syscall.Errno(result)
		switch status {
		case 0:
			return windows.UTF16ToString(buffer), true, nil
		case windows.ERROR_NOT_CONNECTED, windows.ERROR_CONNECTION_UNAVAIL:
			return "", false, nil
		case windows.ERROR_MORE_DATA:
			continue
		default:
			return "", false, status
		}
	}
	return "", false, windows.ERROR_MORE_DATA
}

func windowsIsRemoteDrive(root string) (bool, error) {
	rootName, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return false, err
	}
	return windows.GetDriveType(rootName) == windows.DRIVE_REMOTE, nil
}

func windowsRemoteMatchesEndpoint(remote, endpoint string) bool {
	if remote == endpoint {
		return true
	}
	for _, want := range windowsAcceptedRemotes(endpoint) {
		if strings.EqualFold(strings.TrimSuffix(remote, `\`), strings.TrimSuffix(want, `\`)) {
			return true
		}
	}
	return false
}

// windowsRootEndpoint is the URL net use maps: the capability root plus
// WindowsRootFolder. The endpoint always ends in '/' (see validateEndpoint).
func windowsRootEndpoint(endpoint string) string {
	return strings.TrimSuffix(endpoint, "/") + "/" + WindowsRootFolder
}

// windowsAcceptedRemotes lists the UNC remotes that prove a mapping belongs to
// endpoint. Windows reports the root-folder mapping, and may also report the
// bare capability root, so both are accepted. Both embed the same capability
// token, so either one identifies this mount alone.
func windowsAcceptedRemotes(endpoint string) []string {
	bare, ok := windowsExpectedRemote(endpoint)
	if !ok {
		return nil
	}
	folder, _ := windowsExpectedRemote(windowsRootEndpoint(endpoint))
	return []string{folder, bare}
}

// windowsExpectedRemote returns the UNC remote Windows reports for endpoint once
// net use has mapped it: \\host\DavWWWRoot\path, with the port written as '@'.
// It is computed from the endpoint alone, so the label keys can be written
// before the mapping exists. It returns false when the endpoint has no path.
func windowsExpectedRemote(endpoint string) (string, bool) {
	remainder := strings.TrimPrefix(endpoint, "http://")
	separator := strings.IndexByte(remainder, '/')
	if separator < 0 {
		return "", false
	}
	host := strings.ReplaceAll(remainder[:separator], ":", "@")
	path := strings.ReplaceAll(strings.TrimSuffix(remainder[separator:], "/"), "/", `\`)
	return `\\` + host + `\DavWWWRoot` + path, true
}
