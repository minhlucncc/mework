package docker

import (
	_ "embed"
	"os"
	"path/filepath"
)

//go:embed seccomp-default.json
var seccompProfileJSON string

// writeSeccompProfile writes the embedded seccomp profile to a temp file and
// returns its path. The caller should remove the file when done.
// Returns empty string if the profile data is empty.
func writeSeccompProfile() (string, error) {
	if seccompProfileJSON == "" {
		return "", nil
	}
	dir := filepath.Join(os.TempDir(), "mework-seccomp")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, "seccomp-*.json")
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := f.WriteString(seccompProfileJSON); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}
