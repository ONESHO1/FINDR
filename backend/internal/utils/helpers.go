package utils

import (
	"os"
	"runtime"
	"strings"

	"github.com/ONESHO1/FINDR/backend/internal/log"
)

func GetFileSize(file string) (int64, error) {
	fileInfo, err := os.Stat(file)
	if err != nil {
		log.Logger.WithError(err).WithField("file", file).Error("Failed to get file info (os.Stat)")
		return 0, err
	}

	size := int64(fileInfo.Size())
	return size, nil
}

// removes invalid characters
func RemoveInvalid(title, artist string) (string, string) {
	if runtime.GOOS == "windows" {
		invalidChars := []byte{'<', '>', '<', ':', '"', '\\', '/', '|', '?', '*'}
		for _, invalidChar := range invalidChars {
			title = strings.ReplaceAll(title, string(invalidChar), "")
			artist = strings.ReplaceAll(artist, string(invalidChar), "")
		}
	} else {
		title = strings.ReplaceAll(title, "/", "\\")
		artist = strings.ReplaceAll(artist, "/", "\\")
	}

	return title, artist
}