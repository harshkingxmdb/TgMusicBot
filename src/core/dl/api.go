/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package dl

import (
	"log/slog"

	"ashokshau/tgmusic/config"
	"ashokshau/tgmusic/src/utils"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// apiData provides a unified interface for fetching track and playlist information from various music platforms via an API gateway.
type apiData struct {
	Query    string
	ApiUrl   string
	APIKey   string
	Patterns map[string]*regexp.Regexp
}

var apiPatterns = map[string]*regexp.Regexp{
	utils.Apple:      regexp.MustCompile(`(?i)^https?:\/\/music\.apple\.com\/[a-zA-Z-]+\/(?:song\/(?:[^\/]+\/)?\d+|album\/[^\/]+\/\d+(?:\?i=\d+)?|playlist\/[^\/]+\/pl\.[\w.-]+|artist\/[^\/]+\/\d+)(?:\?.*)?$`),
	utils.Spotify:    regexp.MustCompile(`(?i)^(https?://)?([a-z0-9-]+\.)*spotify\.com/(track|playlist|album|artist)/[a-zA-Z0-9]+(\?.*)?$`),
	utils.JioSaavn:   regexp.MustCompile(`(?i)https?:\/\/(?:www\.)?(?:jiosaavn|saavn)\.com\/(?:s\/)?(song|album|playlist|featured)(?:\/[^\/]+)*\/([A-Za-z0-9_,-]+)(?:\/)?(?:\?.*)?$`),
	utils.Deezer:     regexp.MustCompile(`(?i)https?:\/\/(?:www\.)?deezer\.com\/(?:[a-z]{2}\/)?(track|album|playlist)\/(\d+)`),
	utils.SoundCloud: regexp.MustCompile(`(?i)^(https?://)?(www\.)?soundcloud\.com/[a-zA-Z0-9_-]+/(sets/)?[a-zA-Z0-9._-]+(\?.*)?$`),
	utils.Gaana:      regexp.MustCompile(`(?i)https?:\/\/(?:www\.)?gaana\.com\/(song|album|playlist|artist)\/([A-Za-z0-9\-]+)`),
	utils.Tidal:      regexp.MustCompile(`(?i)https?:\/\/(?:www\.|listen\.)?tidal\.com\/(?:browse\/)?(track|album|playlist)\/([a-zA-Z0-9-]+)(?:[\/?].*)?`),
	utils.MXPlayer:   regexp.MustCompile(`(?i)https?:\/\/(?:www\.)?mxplayer\.in\/(?:show|movie)\/.*`),
	utils.Twitch:     regexp.MustCompile(`(?i)https?:\/\/(?:www\.|m\.)?twitch\.tv\/(?:videos|[\w._-]+\/video)\/\d+`),
	utils.TwitchClip: regexp.MustCompile(
		`(?i)https?:\/\/(?:www\.|m\.)?(?:` +
			`twitch\.tv\/clip\/[\w-]+|` +
			`clips\.twitch\.tv\/[\w-]+|` +
			`twitch\.tv\/[\w-]+\/clip\/[\w-]+` +
			`)`,
	),
	utils.Kick:     regexp.MustCompile(`(?i)https?:\/\/(?:www\.)?kick\.com\/[\w._-]+\/videos\/[a-fA-F0-9-]+`),
	utils.KickClip: regexp.MustCompile(`(?i)https?:\/\/(?:www\.)?kick\.com\/[\w._-]+\/clips\/[\w-]+`),
}

// newApiData creates and initializes a new apiData instance with the provided query.
func newApiData(query string) *apiData {
	return &apiData{
		Query:    strings.TrimSpace(query),
		ApiUrl:   strings.TrimRight(config.ApiUrl, "/"),
		APIKey:   config.ApiKey,
		Patterns: apiPatterns,
	}
}

func (a *apiData) isValid() bool {
	if a.Query == "" || a.ApiUrl == "" || a.APIKey == "" {
		return false
	}

	for _, pattern := range a.Patterns {
		if pattern.MatchString(a.Query) {
			return true
		}
	}
	return false
}

// authHeader builds the Authorization header your nub yt api expects
// (Authorization: Bearer <token>) instead of the old X-API-Key scheme.
func (a *apiData) authHeader() map[string]string {
	return map[string]string{"Authorization": "Bearer " + a.APIKey}
}

// nubStreamResponse mirrors only the fields we need from your nub yt api's
// /stream response — decoding just this instead of the full utils.TrackInfo
// shape avoids relying on unverified field names from a different API.
type nubStreamResponse struct {
	StreamURL string `json:"stream_url"`
}

// getInfo retrieves metadata for a track or playlist from the API.
func (a *apiData) getInfo() (utils.PlatformTracks, error) {
	if !a.isValid() {
		return utils.PlatformTracks{}, errors.New("the provided URL is invalid or the platform is not supported")
	}

	fullURL := fmt.Sprintf("%s/api/get_url?%s", a.ApiUrl, url.Values{"url": {a.Query}}.Encode())
	resp, err := sendRequest(http.MethodGet, fullURL, nil, map[string]string{"X-API-Key": a.APIKey})
	if err != nil {
		return utils.PlatformTracks{}, fmt.Errorf("the GetInfo request failed: %w", err)
	}

	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return utils.PlatformTracks{}, fmt.Errorf("unexpected status code while fetching info: %s", resp.Status)
	}

	var data utils.PlatformTracks
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return utils.PlatformTracks{}, fmt.Errorf("failed to decode the GetInfo response: %w", err)
	}
	return data, nil
}

// search queries the API for a track.
func (a *apiData) search() (utils.PlatformTracks, error) {
	if a.isValid() {
		return a.getInfo()
	}

	fullURL := fmt.Sprintf("%s/api/search?%s", a.ApiUrl, url.Values{
		"query": {a.Query},
		"limit": {"5"},
	}.Encode())

	resp, err := sendRequest(http.MethodGet, fullURL, nil, map[string]string{"X-API-Key": a.APIKey})
	if err != nil {
		return utils.PlatformTracks{}, fmt.Errorf("the search request failed: %w", err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return utils.PlatformTracks{}, fmt.Errorf("unexpected status code during search: %s", resp.Status)
	}

	var data utils.PlatformTracks
	if err = json.NewDecoder(resp.Body).Decode(&data); err != nil {
		slog.Warn("Failed to decode search response", "error", err)
		return utils.PlatformTracks{}, fmt.Errorf("failed to decode the search response: %w", err)
	}

	return data, nil
}

// getTrack retrieves detailed information for a single track from the API.
//
// FIXED for your own nub yt api:
//   - endpoint:    /api/track  -> /stream  (your API doesn't have /api/track;
//     /stream is the one with a confirmed response shape — {"stream_url": "..."})
//   - auth header: X-API-Key   -> Authorization: Bearer <token>
//     (your API's require_token() only accepts a Bearer header or ?token=,
//     never X-API-Key — that mismatch is exactly what caused the 401s in your logs)
//   - response: built manually from a.Query + the resolved stream_url, instead
//     of trusting an unverified JSON shape for utils.TrackInfo's other fields
func (a *apiData) getTrack() (utils.TrackInfo, error) {
	fullURL := fmt.Sprintf("%s/stream?%s", a.ApiUrl, url.Values{"q": {a.Query}, "mode": {"audio"}}.Encode())
	resp, err := sendRequest(http.MethodGet, fullURL, nil, a.authHeader())
	if err != nil {
		slog.Warn("GetTrack request failed", "error", err)
		return utils.TrackInfo{}, fmt.Errorf("the GetTrack request failed: %w", err)
	}

	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	if resp.StatusCode == http.StatusUnauthorized {
		return utils.TrackInfo{}, fmt.Errorf("invalid or missing API token (check API_KEY / config.ApiKey)")
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return utils.TrackInfo{}, fmt.Errorf("daily rate limit exceeded on your API token")
	}
	if resp.StatusCode != http.StatusOK {
		return utils.TrackInfo{}, fmt.Errorf("unexpected status code while fetching the track: %s", resp.Status)
	}

	var stream nubStreamResponse
	if err = json.NewDecoder(resp.Body).Decode(&stream); err != nil {
		slog.Warn("Failed to decode the GetTrack response", "error", err)
		return utils.TrackInfo{}, fmt.Errorf("failed to decode the GetTrack response: %w", err)
	}
	if stream.StreamURL == "" {
		return utils.TrackInfo{}, fmt.Errorf("API returned no stream_url for %s", a.Query)
	}

	return utils.TrackInfo{
		Id:       a.Query,
		URL:      a.Query,
		CdnURL:   stream.StreamURL,
		Platform: utils.YouTube,
	}, nil
}

// downloadTrack downloads a track using the API. If the track is a YouTube video and video format is requested,
func (a *apiData) downloadTrack(info utils.TrackInfo, video bool) (string, error) {
	// if the track is from YouTube and video:true
	yt := newYouTubeData(a.Query)
	if info.Platform == utils.YouTube && video {
		return yt.downloadTrack(info, video)
	}

	downloader, err := newDownload(info)
	if err != nil {
		return "", fmt.Errorf("failed to initialize the download: %w", err)
	}

	filePath, err := downloader.Process()
	if err != nil {
		if info.Platform == utils.YouTube {
			return yt.downloadTrack(info, video)
		}
		return "", fmt.Errorf("the download process failed: %w", err)
	}

	if strings.Contains(a.ApiUrl, filePath) {
		return downloadFile(filePath, "", false)
	}

	return filePath, nil
}
