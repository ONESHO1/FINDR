package spotify

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"

	"github.com/sirupsen/logrus"

	"github.com/ONESHO1/FINDR/backend/internal/config"
	"github.com/ONESHO1/FINDR/backend/internal/log"
)

type Track struct {
	Title, Artist, Album string
	Artists              []string
	Duration             int
}

func spotifyRequest(endpoint string) (int, string, error){
	r, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		log.Logger.WithError(err).Error("Failed to create Spotify request")
		return 0, "", err
	}

	// get access token from spotify
	bearer, err := config.AccessToken()
	if err != nil {
		log.Logger.WithError(err).Error("Failed to get Spotify access token for request")
		return 0, "", err
	}
	r.Header.Add("Authorization", "Bearer " + bearer)

	response, err := (&http.Client{}).Do(r)
	if err != nil {
		log.Logger.WithError(err).WithField("endpoint", endpoint).Error("Failed to execute Spotify request")
		return 0, "", err
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		log.Logger.WithError(err).Error("Failed to read Spotify response body")
		return 0, "", err
	}

	return response.StatusCode, string(responseBody), nil
}

func TrackInfo(link string) (*Track, error) {
	// got ts from gemini
	re := regexp.MustCompile(`open\.spotify\.com\/(?:intl-.+\/)?track\/([a-zA-Z0-9]{22})(\?si=[a-zA-Z0-9]{16})?`)
	matches := re.FindStringSubmatch(link)

	if len(matches) <= 2 {
		err := fmt.Errorf("invalid spotify track url: %s", link)
		log.Logger.WithField("url", link).Warn(err.Error())
		return nil, err
	}

	spotifyID := matches[1]

	spotifyEndpoint := fmt.Sprintf("https://api.spotify.com/v1/tracks/%s", spotifyID)

	statusCode, responseJSON, err := spotifyRequest(spotifyEndpoint)
	if err != nil {
		return nil, err
	}
	if statusCode != 200 {
		err := fmt.Errorf("spotify API returned non-200 status: %d", statusCode)
		log.Logger.WithFields(logrus.Fields{
			"status_code":   statusCode,
			"response_body": responseJSON,
			"endpoint":      spotifyEndpoint,
		}).Error(err)
		return nil, err
	}

	var result struct {
		Name     string `json:"name"`
		Duration int    `json:"duration_ms"`
		Album    struct {
			Name string `json:"name"`
		} `json:"album"`
		Artists []struct {
			Name string `json:"name"`
		} `json:"artists"`
	}

	err = json.Unmarshal([]byte(responseJSON), &result)
	if err != nil {
		log.Logger.WithError(err).WithField("response_json", responseJSON).Error("Failed to unmarshal Spotify track info")
		return nil, err
	}

	var artists []string
	for _, a := range result.Artists {
		artists = append(artists, a.Name)
	}

	// fmt.Println(result)
	log.Logger.WithFields(logrus.Fields{
		"track_title": result.Name,
		"artist":      artists[0],
		"spotify_id":  spotifyID,
	}).Info("Successfully retrieved track info")

	return &Track{
		Title:    result.Name,
		Artist:   artists[0],
		Artists:  artists,
		Album:    result.Album.Name,
		Duration: result.Duration / 1000,
	}, nil
}

func PlaylistInfo(url string) ([]Track, error) {
	re := regexp.MustCompile(`open\.spotify\.com\/playlist\/([a-zA-Z0-9]{22})`)
	matches := re.FindStringSubmatch(url)
	if len(matches) != 2 {
		return nil, errors.New("invalid playlist URL")
	}
	id := matches[1]

	var allTracks []Track
	offset := 0
	limit := 100

	for {
		endpoint := fmt.Sprintf("https://api.spotify.com/v1/playlists/%s/tracks?offset=%d&limit=%d", id, offset, limit)
		statusCode, jsonResponse, err := spotifyRequest(endpoint)
		if err != nil {
			return nil, fmt.Errorf("request error: %w", err)
		}
		if statusCode != 200 {
			return nil, fmt.Errorf("non-200 status: %d", statusCode)
		}

		var result struct {
			Items []struct {
				Track struct {
					Name     string `json:"name"`
					Duration int    `json:"duration_ms"`
					Album    struct {
						Name string `json:"name"`
					} `json:"album"`
					Artists []struct {
						Name string `json:"name"`
					} `json:"artists"`
				} `json:"track"`
			} `json:"items"`
			Total int `json:"total"`
		}
		if err := json.Unmarshal([]byte(jsonResponse), &result); err != nil {
			return nil, err
		}

		for _, item := range result.Items {
			track := item.Track
			var artists []string
			for _, a := range track.Artists {
				artists = append(artists, a.Name)
			}
			allTracks = append(allTracks, *(&Track{
				Title:    track.Name,
				Artist:   artists[0],
				Artists:  artists,
				Duration: track.Duration / 1000,
				Album:    track.Album.Name,
			}).buildTrack())
		}

		offset += limit
		if offset >= result.Total {
			break
		}
	}

	return allTracks, nil
}

func (t *Track) buildTrack() *Track {
	track := &Track{
		Title:    t.Title,
		Artist:   t.Artist,
		Artists:  t.Artists,
		Duration: t.Duration,
		Album:    t.Album,
	}

	return track
}