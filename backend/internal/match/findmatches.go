package match

import (
	"math"
	"math/rand"
	"sort"
	"time"

	"github.com/ONESHO1/FINDR/backend/internal/db"
	fingerprintalgorithm "github.com/ONESHO1/FINDR/backend/internal/fingerprint-algorithm"
	"github.com/ONESHO1/FINDR/backend/internal/log"
)

const TOLERANCE = 100 		// tolerance in time difference

type Match struct {
	SongID     uint32
	SongTitle  string
	SongArtist string
	// YouTubeID  string
	Timestamp  uint32
	Score      float64
}

func FindMatches(sample []float64, duration float64, sampleRate int) ([]Match, time.Duration, error) {
	start := time.Now()

	spectrogram, err := fingerprintalgorithm.Spectrogram(sample, sampleRate)
	if err != nil {
		log.Logger.WithError(err).Error("error finding samples")
		return nil, time.Since(start), err
	} 

	peaks := fingerprintalgorithm.GetPeaksFromSpectrogram(spectrogram, sampleRate)
	sampleFingerprint := fingerprintalgorithm.Fingerprint(peaks, rand.Uint32())

	sampleFingerprintMap := make(map[uint32]uint32)
	for hash, couple := range sampleFingerprint {
		sampleFingerprintMap[hash] = couple.AnchorTimeMs
	}

	matches, err := findMatchesFromDb(sampleFingerprintMap)
	if err != nil {
		log.Logger.WithError(err).Error("error finding matches")
		return nil, time.Since(start), err
	}

	return matches, time.Since(start), nil
}

func findMatchesFromDb(sampleFingerprintMap map[uint32]uint32) ([]Match, error) {
	tmp := make([]uint32, 0, len(sampleFingerprintMap))
	for hash := range sampleFingerprintMap {
		tmp = append(tmp, hash)
	}

	db, err := db.NewDbClient()
	if err != nil {
		log.Logger.WithError(err).Error("error connecting to db")
		return nil, err
	}
	defer db.Close()

	n, err := db.GetCouples(tmp)
	if err != nil {
		log.Logger.WithError(err).Error("couldnt get couples from db")
		return nil, err
	}

	matches := map[uint32][][2]uint32{}        // songID -> [(sampleTime, dbTime)]
	timestamps := map[uint32]uint32{}          // songID -> earliest timestamp
	targetZones := map[uint32]map[uint32]int{} // songID -> timestamp -> count

	for hash, couples := range n {
		for _, couple := range couples {
			matches[couple.SongID] = append(matches[couple.SongID], [2]uint32{sampleFingerprintMap[hash], couple.AnchorTimeMs})

			if existingTime, ok := timestamps[couple.SongID]; !ok || couple.AnchorTimeMs < existingTime {
				timestamps[couple.SongID] = couple.AnchorTimeMs
			}

			if _, ok := targetZones[couple.SongID]; !ok {
				targetZones[couple.SongID] = make(map[uint32]int)
			}

			targetZones[couple.SongID][couple.AnchorTimeMs]++ 
		}
	}

	scores := make(map[uint32]float64)

	/* 
	get the score for each songID from the differences in the recording time and db(saved) time
	I can't get myself to write O(N^3) after doing so many lc qns xD
	*/
	for songID, times := range matches {
		count := 0
		for i := 0 ; i < len(times) ; i++ {
			for j := i + 1 ; j < len(times) ; j++ {
				sampleTimeDiff := math.Abs(float64(times[i][0] - times[j][0]))
				dbTimeDiff := math.Abs(float64(times[i][1] - times[j][1]))
				if math.Abs(sampleTimeDiff - dbTimeDiff) <= TOLERANCE {
					count++
				}
			}
		}
		scores[songID] = float64(count)
	}

	var finalMatches []Match

	for songID, score := range scores {
		song, songExists, err := db.GetSongByID(songID)
		if !songExists {
			log.Logger.Errorf("songID: %d doesnt exist", songID)
			continue
		}
		if err != nil {
			log.Logger.Errorf("couldn't get song by id : %d - %v", songID, err)
			continue
		}
		match := Match{
			SongID: songID,
			SongTitle: song.Title,
			SongArtist: song.Artist,
			Timestamp: timestamps[songID],
			Score: score,
		}
		finalMatches = append(finalMatches, match)
	}

	sort.Slice(finalMatches, func(i, j int) bool{
		return finalMatches[i].Score > finalMatches[j].Score
	})

	return finalMatches, nil
}