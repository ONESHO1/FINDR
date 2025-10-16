package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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

// TODO: maybe change this
func GenerateSongKey(title, artist string) string {
	// Normalize strings to lowercase and trim whitespace
	normalizedTitle := strings.ToLower(strings.TrimSpace(title))
	normalizedArtist := strings.ToLower(strings.TrimSpace(artist))

	// Create a consistent input string, e.g., "the beatles-yesterday"
	input := fmt.Sprintf("%s-%s", normalizedArtist, normalizedTitle)

	// Hash the input string using SHA-256
	hashBytes := sha256.Sum256([]byte(input))

	// Convert the hash to a hexadecimal string for database storage
	return hex.EncodeToString(hashBytes[:])
}