package youtube

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	// "math/rand"

	// "math/rand"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/sirupsen/logrus"

	"github.com/ONESHO1/FINDR/backend/internal/log"
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

// simple http GET request to youtube to | Not using ts, using yt-dlp
func ytSearch(query string, limit int) (results []*SearchResult, err error){
	searchURL := fmt.Sprintf("https://www.youtube.com/results?search_query=%s", url.QueryEscape(query))
	log.Logger.WithField("url", searchURL).Debug("Performing YouTube search")

	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		log.Logger.WithError(err).Error("Failed to create YouTube search request")
		return nil, err
	}
	req.Header.Add("Accept-Language", "en")
	res, err := httpClient.Do(req)
	if err != nil {
		log.Logger.WithError(err).Error("Failed to execute YouTube search request")
		return nil, err
	}

	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(res.Body)

	if res.StatusCode != 200 {
		err = fmt.Errorf("bad status code: %d", res.StatusCode)
		log.Logger.WithField("status_code", res.StatusCode).Error("YouTube search returned non-200 status")
		return nil, err
	}

	buffer, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Logger.WithError(err).Error("Failed to read YouTube search response body")
		return nil, err
	}

	body := string(buffer)
	splitScript := strings.Split(body, `window["ytInitialData"] = `)
	if len(splitScript) != 2 {
		splitScript = strings.Split(body, `var ytInitialData = `)
	}

	if len(splitScript) != 2 {
		err = errors.New("could not find ytInitialData in response body")
		log.Logger.Error(err.Error() + " (YouTube page structure may have changed)")
		return nil, err
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

// use yt-dlp's methods instead of the ytSearch function's simple GET request
func GetYtID(tmpTrack *sp.Track) (string, error) {
	// songDuration := tmpTrack.Duration

	// searchQuery := fmt.Sprintf("'%s' %s", tmpTrack.Title, tmpTrack.Artist)
	searchQuery := fmt.Sprintf("ytsearch1:'%s %s'", tmpTrack.Title, tmpTrack.Artist)

	// // Testing sleeping to avoid youtube's 403 CAPTCHA error
	// time.Sleep(time.Duration(5000+rand.Intn(10000)) * time.Millisecond) // Sleep for 0.5-1.5 seconds

	// only 30 seconds allowed for a search, wlse it DCs
	ctx, cancel := context.WithTimeout(context.Background(), 30 * time.Second)
	defer cancel()

	log.Logger.WithFields(logrus.Fields{
		"track_title": tmpTrack.Title,
		"query":       searchQuery,
	}).Info("Searching for YouTube ID using yt-dlp")

	cmd := exec.CommandContext(ctx, "yt-dlp", "--get-id", searchQuery)

	op, err := cmd.Output()
	if err != nil {
		// logging if there is a error
		if ee, ok := err.(*exec.ExitError); ok {
			log.Logger.WithError(err).WithFields(logrus.Fields{
				"track_title":   tmpTrack.Title,
				"yt-dlp_stderr": string(ee.Stderr),
			}).Error("yt-dlp search failed")
		}
		return "", fmt.Errorf("yt-dlp search failed for query '%s': %w", searchQuery, err)
	}

	ytID := strings.TrimSpace(string(op))
	if ytID == "" {
		return "", fmt.Errorf("yt-dlp did not find a video for: %s", searchQuery)
	}

	log.Logger.WithFields(logrus.Fields{
		"track_title": tmpTrack.Title,
		"youtube_id":  ytID,
	}).Info("Got the YT ID")

	return ytID, nil
}


// using yt-dlp
func DownloadYtAudio(ytID, path, filePath string) (error) {
	dir, err := os.Stat(path)
	if err != nil {
		log.Logger.WithError(err).WithField("path", path).Error("Cannot access download path")
		return err
	}

	if !dir.IsDir() {
		err := fmt.Errorf("path is not a directory: %s", path)
		log.Logger.WithField("path", path).Error(err)
		return err
	}

	var DELAY = 2 * time.Second
	videoURL := "https://www.youtube.com/watch?v=" + ytID

	for i := 0; i < MAX_RETRIES; i++ {
		log.Logger.WithFields(logrus.Fields{
			"attempt":     i + 1,
			"max_retries": MAX_RETRIES,
			"ytID":        ytID,
		}).Info("Attempting to download Auido")
		
		cmd := exec.Command("yt-dlp",
			"--no-playlist",
			// "--cookies-from-browser", "chrome", 		// testing this
			// "--cookies", "cookies.txt",				// testing this
			// "-f", "140", 								// code for 128k M4A audio
			"-f", "bestaudio/best", 					// best m4a audio
			"-o", filePath, 							// Specify the exact output file path and name
			videoURL,
		)

		output, err := cmd.CombinedOutput()
		if err == nil { 		// success
			// verify the file
			fileInfo, checkErr := os.Stat(filePath)
			if checkErr == nil && fileInfo.Size() > 0 {
				log.Logger.WithFields(logrus.Fields{
					"file_path":   filePath,
					"size_bytes":  fileInfo.Size(),
				}).Info("Successfully downloaded and verified audio file")
				return nil 		// Success
			}
		}
		
		// if error
		log.Logger.WithError(err).WithFields(logrus.Fields{
			"ytID":     ytID,
			"retry_in": DELAY,
			"yt-dlp-output": string(output),
		}).Warn("Download attempt failed, retrying...")

		os.Remove(filePath)
		time.Sleep(DELAY)
		// DELAY *= 2	

	}

	finalErr := fmt.Errorf("failed to download video %s after %d attempts", ytID, MAX_RETRIES)
	log.Logger.WithField("ytID", ytID).Error(finalErr)
	return finalErr
}