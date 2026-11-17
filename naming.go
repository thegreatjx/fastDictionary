package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
)

const (
	appName  = "fastDictionary"
	appTitle = "fast词典"
	logFile  = appName + ".log"
)

var legacyDataDirs = []string{"Dictionary", "fast词典"}

func migrateUserData() string {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil || dir == "" {
		return ""
	}
	if _, err := os.Stat(filepath.Join(dir, "config.json")); err == nil {
		return ""
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
