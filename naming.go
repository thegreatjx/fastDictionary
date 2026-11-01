package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
)

// The app answers to two names: fast词典 in the interface, fastDictionary on
// disk. They are not interchangeable — mygo derives the user data directory,
// the log directory and the single instance lock from the executable name
// (Application.Name() falls back to it), so the exe stem, the Go module name
// and appName have to agree. When they disagreed the app wrote its config to
// a directory named after one of them while the user opened a binary named
// after another, and every rename silently orphaned the previous settings.
const (
	appName  = "fastDictionary" // exe stem, user data dir, log prefix
	appTitle = "fast词典"         // what every window and menu shows
	logFile  = appName + ".log"
)

// legacyDataDirs are the user data directories earlier names of this app
// wrote to, best first. They are read once, on the first start under the
// current name, so a rename does not look like data loss.
//
// "Dictionary" comes first because that is where the shortcut-launched build
// kept the user's settings and notebook until the rename; the third entry is
// the directory named after this module's old name, which is what a plain
// `go build .` produced before the rename.
var legacyDataDirs = []string{"Dictionary", "fast词典", "fastpot"}

// migrateUserData copies the settings and the notebook of an earlier name
// into the directory this build uses.
//
// Renaming the executable moves the data directory with it, because mygo
// builds the path from the exe name. Without this the user opens the app
// after an update and finds default settings and an empty notebook, which
// reads as a bug rather than a rename. It copies instead of moving, and only
// when the target has no config yet: a run that already wrote its own data
// must never be overwritten, and leaving the old directory in place makes the
// migration safe to interrupt and repeat.
//
// It returns a line describing what it did, or "" when there was nothing to
// do, so main can log it once the log file is open. Logging from in here
// would write to stderr, and the shipped build has no stderr: the one run
// that migrates the user's data is exactly the run whose record matters.
func migrateUserData() string {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil || dir == "" {
		return ""
	}
	if _, err := os.Stat(filepath.Join(dir, "config.json")); err == nil {
		return "" // this name already has its own data
	}
	appData := os.Getenv("APPDATA")
	if appData == "" {
		return ""
	}
	for _, legacy := range legacyDataDirs {
		src := filepath.Join(appData, legacy)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		n, err := copyUserData(src, dir)
		if err != nil {
			return fmt.Sprintf("%s: could not migrate %s: %v", appName, src, err)
		}
		if n > 0 {
			return fmt.Sprintf("%s: migrated %d file(s) from %s", appName, n, src)
		}
	}
	return ""
}

// copyUserData copies the files worth keeping from a legacy data directory.
// cache/ and logs/ are left behind on purpose: they are disposable, and a
// stale cache copied forward would be indistinguishable from a fresh one.
func copyUserData(src, dst string) (int, error) {
	copied := 0
	for _, name := range []string{"config.json", "vocabulary.json", "notebook.html"} {
		data, err := os.ReadFile(filepath.Join(src, name))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return copied, err
		}
		if err := os.WriteFile(filepath.Join(dst, name), data, 0o644); err != nil {
			return copied, err
		}
		copied++
	}
	return copied, nil
}
