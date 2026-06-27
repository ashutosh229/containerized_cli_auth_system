package cli

import (
	"os"
	"path/filepath"
)

const defaultHistoryFile = ".authcli_history"

func historyFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(home, defaultHistoryFile)
}
