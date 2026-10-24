package ai

import (
	"context"
	"errors"
	"fmt"
	"strconv"
)

// The methods below give fake_drive the item-level operations the agent tools
// use. Ids follow the production shape: a file is its decimal id, a folder is
// "d:<name>" in agent fixtures.

func (d *fake_drive) lookup_locked(item_id string) (Item, bool) {
	if n, err := strconv.ParseInt(item_id, 10, 64); err == nil {
		for _, file := range d.files {
			if file.ID == n {
				return Item{ID: item_id, Name: file.Name, ParentID: file.ParentID, Size: file.Size}, true
			}
		}
		return Item{}, false
	}
	for _, folder := range d.folders {
		if folder.ID == item_id {
			return Item{ID: item_id, Name: folder.Name, IsFolder: true, ParentID: folder.ParentID}, true
		}
	}
	return Item{}, false
}

func (d *fake_drive) Lookup(ctx context.Context, channel_id int64, item_id string) (Item, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	item, ok := d.lookup_locked(item_id)
	if !ok {
		return Item{}, fmt.Errorf("item %s not found in this drive", item_id)
	}
	if d.trashed[item_id] {
		return Item{}, fmt.Errorf("item %s is in the trash", item_id)
	}
	return item, nil
}

// check_write returns the configured write error for an item, if any.
func (d *fake_drive) check_write(op, item_id string) error {
	d.calls = append(d.calls, op+":"+item_id)
	if err := d.write_err[item_id]; err != nil {
		return err
	}
	return nil
}

func (d *fake_drive) RenameItem(ctx context.Context, channel_id int64, item_id string, new_name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.check_write("rename", item_id); err != nil {
		return err
	}
	if n, err := strconv.ParseInt(item_id, 10, 64); err == nil {
		for i := range d.files {
			if d.files[i].ID == n {
				d.files[i].Name = new_name
				return nil
			}
		}
		return errors.New("file not found")
	}
	for i := range d.folders {
		if d.folders[i].ID == item_id {
			d.folders[i].Name = new_name
			return nil
		}
	}
	return errors.New("folder not found")
}

func (d *fake_drive) MoveItem(ctx context.Context, channel_id int64, item_id string, parent_id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.check_write("move", item_id); err != nil {
		return err
	}
	if n, err := strconv.ParseInt(item_id, 10, 64); err == nil {
		for i := range d.files {
			if d.files[i].ID == n {
				d.files[i].ParentID = parent_id
				return nil
			}
		}
		return errors.New("file not found")
	}
	for i := range d.folders {
		if d.folders[i].ID == item_id {
			d.folders[i].ParentID = parent_id
			return nil
		}
	}
	return errors.New("folder not found")
}

func (d *fake_drive) TrashItem(ctx context.Context, channel_id int64, item_id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.check_write("trash", item_id); err != nil {
		return err
	}
	if _, ok := d.lookup_locked(item_id); !ok {
		return errors.New("item not found")
	}
	if d.trashed == nil {
		d.trashed = map[string]bool{}
	}
	d.trashed[item_id] = true
	return nil
}

func (d *fake_drive) RestoreItem(ctx context.Context, channel_id int64, item_id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.trashed[item_id] {
		return errors.New("item is not in the trash")
	}
	if err := d.check_write("restore", item_id); err != nil {
		return err
	}
	delete(d.trashed, item_id)
	return nil
}

// is_trashed is a test helper.
func (d *fake_drive) is_trashed(item_id string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.trashed[item_id]
}

// item_name is a test helper that returns an item's current name.
func (d *fake_drive) item_name(item_id string) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	item, _ := d.lookup_locked(item_id)
	return item.Name
}

// ListTrash lists the trashed files and folders in the order the fixture holds
// them. Item names are the current ones; ParentPath is left empty.
func (d *fake_drive) ListTrash(ctx context.Context, channel_id int64) ([]TrashedItem, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []TrashedItem
	for _, file := range d.files {
		id := fmt.Sprint(file.ID)
		if d.trashed[id] {
			out = append(out, TrashedItem{ID: id, Name: file.Name, Size: file.Size, DeletedAt: 1_700_000_000})
		}
	}
	for _, folder := range d.folders {
		if d.trashed[folder.ID] {
			out = append(out, TrashedItem{ID: folder.ID, Name: folder.Name, IsFolder: true, DeletedAt: 1_700_000_100})
		}
	}
	return out, nil
}

// PurgeItem destroys a trashed item for good: it leaves both the listing and
// the fixture. Purged ids are recorded in d.purged.
func (d *fake_drive) PurgeItem(ctx context.Context, channel_id int64, item_id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.check_write("purge", item_id); err != nil {
		return err
	}
	if !d.trashed[item_id] {
		return errors.New("item is not in the trash")
	}
	if n, err := strconv.ParseInt(item_id, 10, 64); err == nil {
		kept := d.files[:0]
		for _, file := range d.files {
			if file.ID != n {
				kept = append(kept, file)
			}
		}
		d.files = kept
	} else {
		kept := d.folders[:0]
		for _, folder := range d.folders {
			if folder.ID != item_id {
				kept = append(kept, folder)
			}
		}
		d.folders = kept
	}
	delete(d.trashed, item_id)
	d.purged = append(d.purged, item_id)
	return nil
}

// purged_ids is a test helper returning the ids purged so far.
func (d *fake_drive) purged_ids() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.purged...)
}
