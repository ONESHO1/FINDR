package spotify

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"

	log "github.com/sirupsen/logrus"

	"github.com/ONESHO1/FINDR/backend/internal/config"
)

type Track struct {
	Title, Artist, Album string
	Artists              []string
	Duration             int
}

func spotifyRequest(endpoint string) (int, string, error){
	r, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return 0, "", errors.New("error making the request")
	}

	// get access token from spotify
	bearer, err := config.AccessToken()
	if err != nil {
		log.Error(err)
		return 0, "", errors.New("error getting access tokens")
	}
	r.Header.Add("Authorization", "Bearer " + bearer)

	response, err := (&http.Client{}).Do(r)
	if err != nil {
		log.Error(err)
		return 0, "", errors.New("error while getting a response")
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		log.Error(err)
		return 0, "", errors.New("error accessing the body")
	}

	return response.StatusCode, string(responseBody), nil
}

func TrackInfo(link string) (*Track, error) {
	// got ts from gemini
	re := regexp.MustCompile(`open\.spotify\.com\/(?:intl-.+\/)?track\/([a-zA-Z0-9]{22})(\?si=[a-zA-Z0-9]{16})?`)
	matches := re.FindStringSubmatch(link)

	if len(matches) <= 2 {
		return nil, errors.New("INVALID URL")
	}

	spotifyID := matches[1]

	spotifyEndpoint := fmt.Sprintf("https://api.spotify.com/v1/tracks/%s", spotifyID)

	statusCode, responseJSON, err := spotifyRequest(spotifyEndpoint)
	if err != nil {
		return nil, fmt.Errorf("error getting track info: %w", err)
	}
	if statusCode != 200 {
		return nil, fmt.Errorf("non-200 status code: %d", statusCode)
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
		return nil, err
	}

	var artists []string
	for _, a := range result.Artists {
		artists = append(artists, a.Name)
	}

	// fmt.Println(result)

	return &Track{
		Title:    result.Name,
		Artist:   artists[0],
		Artists:  artists,
		Album:    result.Album.Name,
		Duration: result.Duration / 1000,
	}, nil
}