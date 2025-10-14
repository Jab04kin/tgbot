package main

import (
	"os"
	"path/filepath"
	"sync"
)

var (
	dataDir     string
	dataDirOnce sync.Once
)

func getDataDir() string {
	dataDirOnce.Do(func() {
		if v := os.Getenv("DATA_DIR"); v != "" {
			dataDir = v
			return
		}
		// Автодетект стандартных путей Render Disk
		if _, err := os.Stat("/var/data"); err == nil {
			dataDir = "/var/data"
			return
		}
		if _, err := os.Stat("/data"); err == nil {
			dataDir = "/data"
			return
		}
		dataDir = "."
	})
	return dataDir
}

func getDataFilePath(name string) string {
	return filepath.Join(getDataDir(), name)
}

func ensureDataDir() error {
	return os.MkdirAll(getDataDir(), 0755)
}

// readData/readData writeData — абстракция для устойчивого хранения.
// Если заданы GIST_TOKEN и GIST_ID — используем GitHub Gist, иначе файловую систему.
func readData(name string) ([]byte, error) {
    if os.Getenv("GIST_TOKEN") != "" && os.Getenv("GIST_ID") != "" {
        return gistReadFile(name)
    }
    return os.ReadFile(getDataFilePath(name))
}

func writeData(name string, data []byte) error {
    if os.Getenv("GIST_TOKEN") != "" && os.Getenv("GIST_ID") != "" {
        return gistWriteFile(name, data)
    }
    if err := ensureDataDir(); err != nil { return err }
    return os.WriteFile(getDataFilePath(name), data, 0644)
}


