package fingerprintalgorithm

import (
	// "fmt"

	"github.com/ONESHO1/FINDR/backend/internal/log"
	"github.com/sirupsen/logrus"
)

type Couple struct {
	AnchorTimeMs uint32
	SongID       uint32
}

func FingerprintFromSamples(sample []float64, sampleRate int, duration float64, songID uint32) (map[uint32]Couple, error) {
	// spectrogram
	spectrogram, err := Spectrogram(sample, sampleRate)
	if err != nil {
		log.Logger.WithError(err).Error("Can't generate spectrogram")
		return nil, err
	}
	// fmt.Println(spectrogram)
	if len(spectrogram) > 0 {
		log.Logger.WithFields(logrus.Fields{
			"time_bins": len(spectrogram),
			"freq_bins": len(spectrogram[0]),
		}).Debug("Generated spectrogram")
	}
	
	// extract peaks from spectrogram
	peaks := GetPeaksFromSpectrogram(spectrogram, duration)
	log.Logger.WithField("peak_count", len(peaks)).Debug("Extracted peaks from spectrogram")
	// fmt.Println(peaks)

	// get fingerprints from peaks
	fingerprints := Fingerprint(peaks, songID)
	log.Logger.WithField("hash_count", len(fingerprints)).Debug("Created fingerprints from peaks")

	return fingerprints, nil
}