package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/ONESHO1/FINDR/backend/internal/log"
	"github.com/sirupsen/logrus"
)

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

type cachedToken struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

type credentials struct {
	ClientID     string
	ClientSecret string
}

const (
	tokenURL        = "https://accounts.spotify.com/api/token"
	cachedTokenPath = "token.json"
)

func GetEnv(key string, fallback ...string) string {
	if value, ok := os.LookupEnv(key); ok {
		// fmt.Println(value)
		return value
	}
	if len(fallback) > 0 {
		return fallback[0]
	}
	return ""
}


func loadCredentials() (*credentials, error) {
	clientID := GetEnv("SPOTIFY_CLIENT_ID", "")
	clientSecret := GetEnv("SPOTIFY_CLIENT_SECRET", "")

	if clientID == "" || clientSecret == "" {
		err := errors.New("SPOTIFY_CLIENT_ID or SPOTIFY_CLIENT_SECRET environment variables not set")
		log.Logger.Error(err)
		return nil, err
	}

	return &credentials{
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}, nil
}

func loadCachedToken() (string, error) {
	data, err := os.ReadFile(cachedTokenPath)
	if err != nil {
		log.Logger.WithError(err).Debug("Could not read cached token file")
		return "", err
	}
	var ct cachedToken
	if err := json.Unmarshal(data, &ct); err != nil {
		log.Logger.WithError(err).Warn("Could not unmarshal cached token file")
		return "", err
	}
	if time.Now().After(ct.ExpiresAt) {
		log.Logger.Debug("Cached token has expired")
		return "", errors.New("token expired")
	}
	return ct.Token, nil
}

func saveToken(token string, expiresIn int) error {
	ct := cachedToken{
		Token:     token,
		ExpiresAt: time.Now().Add(time.Duration(expiresIn) * time.Second),
	}
	data, err := json.MarshalIndent(ct, "", "  ")
	if err != nil {
		log.Logger.WithError(err).Error("Failed to marshal token for saving")
		return err
	}
	return os.WriteFile(cachedTokenPath, data, 0644)
}

func AccessToken() (string, error) {
	// Try using cached token
	token, err := loadCachedToken()
	if err == nil {
		log.Logger.Info("Using cached Spotify access token")
		return token, nil
	}

	// Fallback: request a new token
	creds, err := loadCredentials()
	if err != nil {
		return "", err
	}

	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", creds.ClientID)
	data.Set("client_secret", creds.ClientSecret)

	req, err := http.NewRequest("POST", tokenURL, bytes.NewBufferString(data.Encode()))
	if err != nil {
		log.Logger.WithError(err).Error("Failed to create token request")
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Logger.WithError(err).Error("Failed to execute token request")
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		err := fmt.Errorf("token request failed with status %d", resp.StatusCode)
		log.Logger.WithFields(logrus.Fields{
			"status_code":   resp.StatusCode,
			"response_body": string(body),
		}).Error("Token request failed")
		return "", err
	}

	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		log.Logger.WithError(err).Error("Failed to decode token response body")
		return "", err
	}

	if err := saveToken(tr.AccessToken, tr.ExpiresIn); err != nil {
		return "", err
	}
	
	log.Logger.Info("Successfully retrieved and saved new Spotify access token")
	return tr.AccessToken, nil
}