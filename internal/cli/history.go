package cli

import (
	"os"
	"path/filepath"
)

func historyFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(home, ".authcli_history")
}
