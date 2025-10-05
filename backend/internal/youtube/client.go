package youtube

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/kkdai/youtube/v2"
	log "github.com/sirupsen/logrus"

	sp "github.com/ONESHO1/FINDR/backend/internal/spotify"
)

type SearchResult struct {
	Title, Uploader, URL, Duration, ID string
	Live                               bool
	SourceName                         string
	Extra                              []string
}

var httpClient = &http.Client{}
const DURATION_THRESHOLD = 5
const MAX_RETRIES = 20

func getContent(data []byte, index int) []byte {
	id := fmt.Sprintf("[%d]", index)
	contents, _, _, _ := jsonparser.Get(data, "contents", "twoColumnSearchResultsRenderer", "primaryContents", "sectionListRenderer", "contents", id, "itemSectionRenderer", "contents")
	return contents
}

func convertStringDurationToSeconds(durationStr string) int {
	splitEntities := strings.Split(durationStr, ":")
	if len(splitEntities) == 1 {
		seconds, _ := strconv.Atoi(splitEntities[0])
		return seconds
	} else if len(splitEntities) == 2 {
		seconds, _ := strconv.Atoi(splitEntities[1])
		minutes, _ := strconv.Atoi(splitEntities[0])
		return (minutes * 60) + seconds
	} else if len(splitEntities) == 3 {
		seconds, _ := strconv.Atoi(splitEntities[2])
		minutes, _ := strconv.Atoi(splitEntities[1])
		hours, _ := strconv.Atoi(splitEntities[0])
		return ((hours * 60) * 60) + (minutes * 60) + seconds
	} else {
		return 0
	}
}

// simple http GET request to youtube to 
func ytSearch(query string, limit int) (results []*SearchResult, err error){
	searchURL := fmt.Sprintf("https://www.youtube.com/results?search_query=%s", url.QueryEscape(query))

	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, errors.New("cannot get youtube page")
	}
	req.Header.Add("Accept-Language", "en")
	res, err := httpClient.Do(req)
	if err != nil {
		return nil, errors.New("cannot get youtube page")
	}

	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(res.Body)

	if res.StatusCode != 200 {
		return nil, errors.New("failed to make a request to youtube")
	}

	buffer, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, errors.New("cannot read response from youtube")
	}

	body := string(buffer)
	splitScript := strings.Split(body, `window["ytInitialData"] = `)
	if len(splitScript) != 2 {
		splitScript = strings.Split(body, `var ytInitialData = `)
	}

	if len(splitScript) != 2 {
		return nil, errors.New("invalid response from youtube")
	}
	splitScript = strings.Split(splitScript[1], `window["ytInitialPlayerResponse"] = null;`)
	jsonData := []byte(splitScript[0])

	index := 0
	var contents []byte

	for {
		contents = getContent(jsonData, index)
		_, _, _, err = jsonparser.Get(contents, "[0]", "carouselAdRenderer")

		if err == nil {
			index++
		} else {
			break
		}
	}

	_, err = jsonparser.ArrayEach(contents, func(value []byte, t jsonparser.ValueType, i int, err error) {
		if err != nil {
			return
		}

		if limit > 0 && len(results) >= limit {
			return
		}

		id, err := jsonparser.GetString(value, "videoRenderer", "videoId")
		if err != nil {
			return
		}

		title, err := jsonparser.GetString(value, "videoRenderer", "title", "runs", "[0]", "text")
		if err != nil {
			return
		}

		uploader, err := jsonparser.GetString(value, "videoRenderer", "ownerText", "runs", "[0]", "text")
		if err != nil {
			return
		}

		live := false
		duration, err := jsonparser.GetString(value, "videoRenderer", "lengthText", "simpleText")

		if err != nil {
			duration = ""
			live = true
		}

		results = append(results, &SearchResult{
			Title:      title,
			Uploader:   uploader,
			Duration:   duration,
			ID:         id,
			URL:        fmt.Sprintf("https://youtube.com/watch?v=%s", id),
			Live:       live,
			SourceName: "youtube",
		})
	})

	if err != nil {
		return results, err
	}

	return results, nil
}

func GetYtID(tmpTrack *sp.Track) (string, error) {
	songDuration := tmpTrack.Duration

	searchQuery := fmt.Sprintf("'%s' %s", tmpTrack.Title, tmpTrack.Artist)

	searchResults, err := ytSearch(searchQuery, 10)
	if err != nil {
		return "", err
	}
	if len(searchResults) == 0 {
		err = fmt.Errorf("no songs found for %s", searchQuery)
		return "", err
	}

	// return the option with the closest matching timestamp
	for _, res := range searchResults {
		allowedStart := songDuration - DURATION_THRESHOLD
		allowedEnd := songDuration + DURATION_THRESHOLD

		resultDuration := convertStringDurationToSeconds(res.Duration)
		if resultDuration >= allowedStart && resultDuration <= allowedEnd {
			log.Info("INFO: ", fmt.Sprintf("Found song with id '%s'", res.ID))
			return res.ID, nil
		}
	}

	return "", fmt.Errorf("could not settle on a song from search result for: %s", searchQuery)
}


// "github.com/kkdai/youtube/v2"
func DownloadYtAudio(ytID, path, filePath string) (error) {
	dir, err := os.Stat(path)
	if err != nil {
		log.Error(errors.New("error accessing path"))
		return err
	}

	if !dir.IsDir() {
		err := errors.New("the path is not valid (not a dir)")
		log.Error("Invalid directory path")
		return err
	}

	var DELAY = 2 * time.Second

	for i := 0; i < MAX_RETRIES; i++ {
		log.Info(fmt.Sprintf("Download attempt %d/%d for ytID: %s", i, MAX_RETRIES, ytID))
		

		err = func() error {
			client := youtube.Client{}

			video, err := client.GetVideo(ytID)
			// fmt.Println(video)
			if err != nil {
				return fmt.Errorf("error getting video metadata: %w", err)
			}

			/*
			itag code: 140, container: m4a, content: audio, bitrate: 128k
			change the FindByItag parameter to 139 if you want smaller files (but with a bitrate of 48k)
			https://gist.github.com/sidneys/7095afe4da4ae58694d128b1034e01e2
			*/
			formats := video.Formats.Itag(140)
			if len(formats) == 0 {
				return fmt.Errorf("no suitable audio format (itag 140) found")
			}

			stream, size, err := client.GetStream(video, &formats[0])
			println(size)
			if err != nil {
				return fmt.Errorf("error getting stream: %w", err)
			}
			defer stream.Close()

			file, err := os.Create(filePath)
			fmt.Print("CREATED FILE \n\n")
			if err != nil {
				return fmt.Errorf("error creating file: %w", err)
			}
			defer file.Close()

			_, err = io.Copy(file, stream)
			if err != nil {
				return fmt.Errorf("error copying stream to file: %w", err)
			}

			fileInfo, err := file.Stat()
			if err == nil && fileInfo.Size() > 0 {
				log.Infof("Successfully downloaded '%s' to '%s'", video.Title, filePath)
				return nil // success
			}
			
			return errors.New("download completed but file is empty")
		}()
		
		if err == nil {
			return nil
		}

		// if error
		log.Errorf("%v. Retrying in %v", err, DELAY)
		os.Remove(filePath)
		time.Sleep(DELAY)
		DELAY *= 2		

	}

	return fmt.Errorf("failed to download video %s after %d attempts. RETRY AFTER SOME TIME", ytID, MAX_RETRIES)

}