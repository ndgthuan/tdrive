//go:build windows

package mountos

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	windows_mount_points_path = `Software\Microsoft\Windows\CurrentVersion\Explorer\MountPoints2`
	// windows_label_value_name is the MountPoints2 value Explorer reads as the
	// display label for a mapped network drive. It overrides the server-derived
	// name without changing the mapped remote or the endpoint capability.
	windows_label_value_name = "_LabelFromReg"
	// shchangenotify_assocchanged is SHCNE_ASSOCCHANGED. With no item IDs
	// (SHCNF_IDLIST = 0) it asks Explorer to re-read association and label
	// state, so the new label appears without a manual refresh.
	shchangenotify_assocchanged = 0x08000000
	// The drive-scoped events below take a path in dwItem1 (SHCNF_PATHW). They
	// only make Explorer refresh views of the drive root; they change no state.
	shchangenotify_attributes = 0x00000800 // SHCNE_ATTRIBUTES
	shchangenotify_updatedir  = 0x00001000 // SHCNE_UPDATEDIR
	shchangenotify_updateitem = 0x00002000 // SHCNE_UPDATEITEM
	shchangenotify_driveadd   = 0x00000100 // SHCNE_DRIVEADD
	shchangenotify_pathw      = 0x00000005 // SHCNF_PATHW
)

var shell_change_notify = windows.NewLazySystemDLL("shell32.dll").NewProc("SHChangeNotify")

// windows_set_mount_label writes the Explorer label for the mapping whose
// remote is remote, to every MountPoints2 subkey variant of that remote (see
// build_label_key_variants). It creates the subkeys if Windows has not created
// them. It attempts every variant and returns the first error.
func windows_set_mount_label(remote, label string) error {
	names, err := build_label_key_variants(remote)
	if err != nil {
		return err
	}
	var first error
	for _, name := range names {
		if err := set_mount_label_value(name, label); err != nil && first == nil {
			first = err
		}
	}
	notify_shell_association_changed()
	return first
}

// windows_clear_mount_label deletes every MountPoints2 subkey variant created
// for remote. The subkey names embed the per-mount capability, so they belong
// to this mapping alone. Missing keys are success.
func windows_clear_mount_label(remote string) error {
	names, err := build_label_key_variants(remote)
	if err != nil {
		return err
	}
	var first error
	for _, name := range names {
		err := registry.DeleteKey(registry.CURRENT_USER, windows_mount_points_path+`\`+name)
		if err != nil && !errors.Is(err, registry.ErrNotExist) && first == nil {
			first = err
		}
	}
	notify_shell_association_changed()
	return first
}

func set_mount_label_value(name, label string) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, windows_mount_points_path+`\`+name, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	return key.SetStringValue(windows_label_value_name, label)
}

// notify_shell_association_changed is best effort: a failed notification only
// delays Explorer's refresh, so its result is ignored.
func notify_shell_association_changed() {
	_, _, _ = shell_change_notify.Call(shchangenotify_assocchanged, 0, 0, 0)
}

// windows_notify_drive_changed asks Explorer to refresh the views of the drive
// root (for example `T:\`) after its label was written. It is best effort:
// every notification is attempted and all errors are ignored.
func windows_notify_drive_changed(root string) {
	path, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return
	}
	for _, event := range []uintptr{
		shchangenotify_updateitem,
		shchangenotify_updatedir,
		shchangenotify_attributes,
		shchangenotify_driveadd,
	} {
		_, _, _ = shell_change_notify.Call(event, shchangenotify_pathw, uintptr(unsafe.Pointer(path)), 0)
	}
	notify_shell_association_changed()
}
