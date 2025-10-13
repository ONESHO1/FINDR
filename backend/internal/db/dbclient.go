package db

import (
	"errors"
	"fmt"

	"github.com/ONESHO1/FINDR/backend/internal/config"
	"github.com/ONESHO1/FINDR/backend/internal/fingerprint-algorithm"
	"github.com/ONESHO1/FINDR/backend/internal/log"
)

type DbClient interface {
	Close() error
	StoreFingerprints(fingerprints map[uint32]fingerprintalgorithm.Couple) error
	GetCouples(addresses []uint32) (map[uint32][]fingerprintalgorithm.Couple, error)
	TotalSongs() (int, error)
	RegisterSong(songTitle, songArtist string) (uint32, error)
	GetSong(filterKey string, value interface{}) (Song, bool, error)
	GetSongByID(songID uint32) (Song, bool, error)
	GetSongByKey(key string) (Song, bool, error)
	DeleteSongByID(songID uint32) error
	DeleteCollection(collectionName string) error
}

type Song struct {
	Title     string
	Artist    string
	YouTubeID string
}

func NewDbClient() (DbClient, error) {
	username := config.GetEnv("POSTGRES_USERNAME", "")
	password := config.GetEnv("POSTGRES_PASSWORD", "")
	dbName := config.GetEnv("POSTGRES_DATABASE_NAME", "")

	if username == "" || password == "" || dbName == "" {
		err := errors.New("POSTGRES environment variables not set")
		log.Logger.Error(err)
		return nil, err
	}
	return newPostgresClient(fmt.Sprintf("postgres://%s:%s@localhost:5432/%s", username, password, dbName))
}