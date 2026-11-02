package file

import (
	"context"
	"fmt"

	"TDrive/backend/projection"
	"TDrive/backend/services/servicecontext"
)

// ReplaceImported puts the newly uploaded file newMsgID in place of the older
// file oldMsgID, under name. The old file goes to the trash, where RestoreObject
// can bring it back, and then the new file takes the name. The new content is
// uploaded before anything moves, so a failure part way leaves it visible under
// its copy name instead of losing it. The new file must still be live before the
// old one is trashed, so a copy that vanished never leaves neither file live.
func (s *Service) ReplaceImported(ctx context.Context, channelID, oldMsgID, newMsgID int64, name string) error {
	if err := servicecontext.Check(ctx, "file: replace imported"); err != nil {
		return err
	}
	if err := s.ready(); err != nil {
		return err
	}
	if channelID == 0 {
		return fmt.Errorf("No active channel")
	}
	if oldMsgID <= 0 || newMsgID <= 0 || oldMsgID == newMsgID {
		return fmt.Errorf("replace needs two different files")
	}
	newEntry, newFound, err := projection.DirentByID(s.DB, channelID, fmt.Sprintf("%s%d", projection.FileIDPrefix, newMsgID))
	if err != nil {
		return err
	}
	if !newFound || newEntry.Tombstoned {
		return fmt.Errorf("replace imported: the new copy is not on the drive")
	}
	objectID := fmt.Sprintf("%s%d", projection.FileIDPrefix, oldMsgID)
	entry, found, err := projection.DirentByID(s.DB, channelID, objectID)
	if err != nil {
		return err
	}
	// An old file that is already gone needs no trash; the name is free anyway.
	if found && !entry.Tombstoned {
		if err := s.TrashObject(ctx, channelID, objectID); err != nil {
			return err
		}
	}
	return s.Rename(ctx, channelID, int(newMsgID), name)
}
