package songdownload

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	
	"github.com/sirupsen/logrus"

	"github.com/ONESHO1/FINDR/backend/internal/log"
	sp "github.com/ONESHO1/FINDR/backend/internal/spotify"
	"github.com/ONESHO1/FINDR/backend/internal/utils"
	yt "github.com/ONESHO1/FINDR/backend/internal/youtube"
	"github.com/ONESHO1/FINDR/backend/internal/wav"
	"github.com/ONESHO1/FINDR/backend/internal/fingerprint-algorithm"
)

const SONGS_DIRECTORY string = "songs"


func download(tracks []sp.Track, path string) (error) {
	/* WaitGroup is a synchronization tool used 
	to wait for a collection of goroutines to finish.
	TS is basically a counter.
	*/
	var wg sync.WaitGroup
	// var downloadedTracks []string
	var NoOfTracks int = 0

	/* chan = buffered channel
	basically a communication pipe that allows different goroutines to send and receive data safely
	*/

	// channel to record number of tracks we successfully downloaded
	results := make(chan int, len(tracks))

	// get number of cores in our CPU
	numCPUs := runtime.NumCPU()
	// fmt.Println(numCPUs)

	/*
	A classic Go idiom for creating a semaphore. 
	A semaphore is used to limit the number of goroutines that can run a piece of code at the same time.

	It's a buffered channel of struct{}. 
	An empty struct (struct{}) is used because it takes up zero memory; 
	we only care about the blocking behavior of the channel, not the data being sent.

	By setting the size to numCPUs, 
	we ensure that a maximum of numCPUs goroutines can "acquire" a slot in the semaphore at any given time. 
	Any other goroutine trying to acquire a slot will block until one is freed.
	*/
	semaphore := make(chan struct{}, numCPUs)

	// TODO: establish DB connection

	for _, t := range tracks {
		// add to WaitGroup
		wg.Add(1)

		// start a goroutine (lightweight thread managed by the Go runtime for concurrency)
		// also need to pass t as a parameter so we create a copy, if we pass t directly, the last t from tracks will be the value seen
		go func(track sp.Track) {
			// defer -> schedules a function call to be run just before the current function returns.
			// wg.Done -> decrement WaitGroup by 1
			defer wg.Done()

			// acquire a slot in the semaphore
			semaphore <- struct{}{}
			// release a slot in the semaphore at the end
			defer func() {
				<- semaphore
			}()

			tmpTrack := &sp.Track{
				Album:    track.Album,
				Artist:   track.Artist,
				Artists:  track.Artists,
				Duration: track.Duration,
				Title:    track.Title,
			}

			// TODO: check if song exists in DB

			// Get youtube ID
			ytID, err := yt.GetYtID(tmpTrack)
			if ytID == "" || err != nil{
				log.Logger.WithFields(logrus.Fields{
					"title":  tmpTrack.Title,
					"artist": tmpTrack.Artist,
					"error":  err,
				}).Error("Could not get YouTube ID for track")
				return
			}
			// fmt.Println(ytID)
			
			tmpTrack.Title, tmpTrack.Artist = utils.RemoveInvalid(tmpTrack.Title, tmpTrack.Artist)
			fileName := fmt.Sprintf("%s - %s", tmpTrack.Title, tmpTrack.Artist)
			filePath := filepath.Join(path, fileName + ".m4a")

			// download audio file from youtube
			err = yt.DownloadYtAudio(ytID, path, filePath)
			if err != nil {
				log.Logger.WithFields(logrus.Fields{
					"title": tmpTrack.Title,
					"artist": tmpTrack.Artist,
					"ytID": ytID,
					"error": err,
				}).Error("Could not download youtube audio")
				return
			}

			// convert to wav file (single channel)
			wavFilePath, err := wav.ConvertToWav(filePath, 1)
			if err != nil {
				log.Logger.WithFields(logrus.Fields{
					"title":  tmpTrack.Title,
					"artist": tmpTrack.Artist,
					"file":   filePath,
					"error":  err,
				}).Error("Processing failed at WAV conversion step")
				return
			}

			// read the wav file info
			wavInfo, err := wav.WavInfo(wavFilePath)
			if err != nil {
				log.Logger.WithFields(logrus.Fields{
					"title":  tmpTrack.Title,
					"artist": tmpTrack.Artist,
					"file":   filePath,
					"wav file": wavFilePath,
					"error":  err,
				}).Error("Could'nt get the WAV info from header")
				return
			}

			// convert the wav bytes into samples
			samples, err := wav.Samples(wavInfo.Data)
			if err != nil {
				log.Logger.WithFields(logrus.Fields{
					"title":  tmpTrack.Title,
					"artist": tmpTrack.Artist,
					"file":   filePath,
					"wav file": wavFilePath,
					"error":  err,
				}).Error("Error converting WAV bytes to samples")
				return
			}
			// fmt.Println(samples)


			// Register songs	
			// songID, err := dbClient.RegisterSong(tmpTrack.Title, tmpTrack.Artist, ytID)
			var songID uint32 = 20 

			// fingerprint song
			fingerprint, err := fingerprintalgorithm.FingerprintFromSamples(samples, wavInfo.SampleRate, wavInfo.Duration, songID)
			if err != nil {
				log.Logger.WithFields(logrus.Fields{
					"title":  tmpTrack.Title,
					"artist": tmpTrack.Artist,
					"error":  err,
				}).Error("Processing failed at fingerprinting step")

				// delete songID
				return
			}
			// fmt.Println(fingerprint)
			// tmp
			log.Logger.WithFields(logrus.Fields{
				"title":            tmpTrack.Title,
				"artist":           tmpTrack.Artist,
				"fingerprint count": len(fingerprint),
			}).Info("Successfully generated fingerprints for track")

			// store fingerprints

			// delete file

			results <- 1
		}(t)
	}

	// must be inside a new goroutine
	go func() {
		/*
		This blocks the new goroutine until the WaitGroup counter becomes zero. 
		It effectively pauses here until all the download goroutines have called wg.Done().
		*/
		wg.Wait()
		// close the results channel
		close(results)
	}()

	for range results {
		NoOfTracks++
	}

	log.Logger.WithField("count", NoOfTracks).Info("Finished download process")
	return nil
}

func downloadTrack(link string, path string) (error) {
	// get track info
	log.Logger.Info("Getting Track Info")
	trackInfo, err := sp.TrackInfo(link)
	if err != nil {
		// fmt.Println("Could not get track's info")
		// log.Error(err)
		log.Logger.WithError(err).WithField("link", link).Error("Could not get track's info")
		return err
	}

	// fmt.Println(trackInfo)
	// list of tracks with a single track
	track := []sp.Track{*trackInfo}

	log.Logger.Info("Downloading Track")
	err = download(track, path)
	if err != nil {
		return err
	}

	return nil
}

func GetSongFromSpotify(spotifyLink string) {
	err := os.MkdirAll(SONGS_DIRECTORY, 0755)
	if err != nil {
		log.Logger.WithError(err).WithField("directory", SONGS_DIRECTORY).Error("Could not create songs directory")
		return
	}

	if strings.Contains(spotifyLink, "track") {
		err = downloadTrack(spotifyLink, SONGS_DIRECTORY)
		if err != nil {
			log.Logger.WithError(err).Error("The download process failed")
		}
	} else {
		log.Logger.WithField("url", spotifyLink).Warn("Invalid Spotify URL: expected a track link")
	}
}