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
	var wg sync.WaitGroup
	// var downloadedTracks []string
	var NoOfTracks int = 0

	results := make(chan int, len(tracks))
	numCPUs := runtime.NumCPU()
	semaphore := make(chan struct{}, numCPUs)

	// TODO: establish DB connection

	for _, t := range tracks {
		wg.Add(1)
		go func(track sp.Track) {
			defer wg.Done()

			semaphore <- struct{}{}
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
		}(t)
	}

	go func() {
		wg.Wait()
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