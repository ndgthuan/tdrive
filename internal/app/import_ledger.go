package app

import (
	"path/filepath"

	"TDrive/backend/datadir"
	"TDrive/backend/importledger"
)

// ImportLedgerVeto reports, for each entry, whether the ledger forbids skipping
// it even though live state matched. Only an edited file (a known modification
// time that differs from the recorded one) is vetoed.
func (a *App) ImportLedgerVeto(driveID int64, parentID string, entries []importledger.Entry) ([]bool, error) {
	ledger, err := a.importLedgerStore()
	if err != nil {
		return nil, err
	}
	return ledger.Veto(driveID, parentID, entries), nil
}

// ImportLedgerRecord remembers that these entries uploaded to parentID on
// driveID. Pass only files whose staged bytes were confirmed uploaded. An empty
// list retries a save that failed earlier.
func (a *App) ImportLedgerRecord(driveID int64, parentID string, entries []importledger.Entry) error {
	ledger, err := a.importLedgerStore()
	if err != nil {
		return err
	}
	return ledger.Record(driveID, parentID, entries)
}

// ImportLedgerForget drops everything remembered for driveID.
func (a *App) ImportLedgerForget(driveID int64) error {
	ledger, err := a.importLedgerStore()
	if err != nil {
		return err
	}
	return ledger.Forget(driveID)
}

func (a *App) importLedgerStore() (*importledger.Ledger, error) {
	a.importLedgerOnce.Do(func() {
		dir, err := datadir.Dir()
		if err != nil {
			a.importLedgerErr = err
			return
		}
		a.importLedger = importledger.New(filepath.Join(dir, importledger.DirName))
	})
	return a.importLedger, a.importLedgerErr
}
