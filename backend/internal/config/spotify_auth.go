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
		return nil, fmt.Errorf("SPOTIFY_CLIENT_ID or SPOTIFY_CLIENT_SECRET environment variables not set")
	}

	return &credentials{
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}, nil
}

func loadCachedToken() (string, error) {
	data, err := os.ReadFile(cachedTokenPath)
	if err != nil {
		return "", err
	}
	var ct cachedToken
	if err := json.Unmarshal(data, &ct); err != nil {
		return "", err
	}
	if time.Now().After(ct.ExpiresAt) {
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
		return err
	}
	return os.WriteFile(cachedTokenPath, data, 0644)
}

func AccessToken() (string, error) {
	// Try using cached token
	token, err := loadCachedToken()
	if err == nil {
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
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", errors.New("token request failed (have a look at credentials.json): " + string(body))
	}

	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", err
	}

	if err := saveToken(tr.AccessToken, tr.ExpiresIn); err != nil {
		return "", err
	}

	return tr.AccessToken, nil
}