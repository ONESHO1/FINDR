package songdownload

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	
	log "github.com/sirupsen/logrus"

	sp "github.com/ONESHO1/FINDR/backend/internal/spotify"
	"github.com/ONESHO1/FINDR/backend/internal/utils"
	yt "github.com/ONESHO1/FINDR/backend/internal/youtube"
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
				log.Error(fmt.Sprintf("'%s' by '%s' could not be downloaded (inside download())", tmpTrack.Title, tmpTrack.Artist))
				return 
			}
			// fmt.Println(ytID)
			
			tmpTrack.Title, tmpTrack.Artist = utils.RemoveInvalid(tmpTrack.Title, tmpTrack.Artist)
			fileName := fmt.Sprintf("%s - %s", tmpTrack.Title, tmpTrack.Artist)
			filePath := filepath.Join(path, fileName + ".m4a")

			// download audio file from youtube
			err = yt.DownloadYtAudio(ytID, path, filePath)
			if err != nil {
				log.Error(fmt.Sprintf("%s by %s could not be downloaded", tmpTrack.Title, tmpTrack.Artist))
				log.Error(err)
				return
			}

			// fingerprint song

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

	log.Info(fmt.Sprintf("Total tracks downloaded: %d", NoOfTracks))
	return nil
}

func downloadTrack(link string, path string) (error) {
	// get track info
	log.Info("Getting Track Info")
	trackInfo, err := sp.TrackInfo(link)
	if err != nil {
		fmt.Println("Could not get track's info")
		log.Error(err)
	}

	// fmt.Println(trackInfo)
	// list of tracks with a single track
	track := []sp.Track{*trackInfo}

	log.Info("Downloading Track")
	err = download(track, path)
	if err != nil {
		return err
	}

	return nil
}

func GetSongFromSpotify(spotifyLink string) (){
	err := os.MkdirAll(SONGS_DIRECTORY, 0755)
	if err != nil {
		fmt.Println("Could not create SONGS DIRECTORY")
		log.Error(err)
	}

	if strings.Contains(spotifyLink, "track") {
		err = downloadTrack(spotifyLink, SONGS_DIRECTORY)
		if err != nil {
			fmt.Println("Could not download track")
			log.Error(err)
		}
	} else {
		fmt.Println("expected single track")
		fmt.Println("INVALID URL")
	}
}