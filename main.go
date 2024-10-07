package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/grafov/m3u8"
)

func main() {
	masterManifestURLStr := os.Args[1]
	headers := map[string]string{
		"User-Agent": "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36",
	}

	masterManifestURL, err := url.Parse(masterManifestURLStr)
	if err != nil {
		fmt.Printf("Error parsing URL %s: %v\n", masterManifestURLStr, err)
		return
	}

	// Fetch and parse the master manifest
	masterPlaylist, err := fetchAndParseMasterManifest(masterManifestURL, headers)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Pick the minimal bandwidth variant
	variant := pickMinimalBandwidthVariant(masterPlaylist)

	// Fetch and parse the media playlist
	mediaPlaylistURL, err := masterManifestURL.Parse(variant.URI)
	if err != nil {
		fmt.Printf("Error parsing media playlist URL: %v\n", err)
		return
	}

	mediaPlaylist, err := fetchAndParseMediaPlaylist(mediaPlaylistURL, headers)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Download and handle the segments
	for _, segment := range mediaPlaylist.Segments {
		if segment == nil {
			continue
		}
		if err := downloadAndHandleSegment(mediaPlaylistURL, segment, headers); err != nil {
			fmt.Println(err)
			return
		}
	}
}

func fetchAndParseMasterManifest(url *url.URL, headers map[string]string) (*m3u8.MasterPlaylist, error) {
	resp, err := makeHTTPRequest("GET", url.String(), headers)
	if err != nil {
		return nil, fmt.Errorf("Error making request: %v", err)
	}
	defer resp.Body.Close()

	playlist, listType, err := m3u8.DecodeFrom(resp.Body, true)
	if err != nil {
		return nil, fmt.Errorf("Error parsing manifest: %v", err)
	}
	if listType != m3u8.MASTER {
		return nil, fmt.Errorf("Not a master playlist")
	}

	return playlist.(*m3u8.MasterPlaylist), nil
}

func fetchAndParseMediaPlaylist(url *url.URL, headers map[string]string) (*m3u8.MediaPlaylist, error) {
	resp, err := makeHTTPRequest("GET", url.String(), headers)
	if err != nil {
		return nil, fmt.Errorf("Error making request to download media playlist: %v", err)
	}
	defer resp.Body.Close()

	playlist, listType, err := m3u8.DecodeFrom(resp.Body, true)
	if err != nil {
		return nil, fmt.Errorf("Error parsing media playlist: %v", err)
	}
	if listType != m3u8.MEDIA {
		return nil, fmt.Errorf("Not a media playlist")
	}

	return playlist.(*m3u8.MediaPlaylist), nil
}

func makeHTTPRequest(method, url string, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("Error creating request: %v", err)
	}

	for key, value := range headers {
		req.Header.Add(key, value)
	}

	client := &http.Client{}
	return client.Do(req)
}

func pickMinimalBandwidthVariant(masterPlaylist *m3u8.MasterPlaylist) *m3u8.Variant {
	variant := masterPlaylist.Variants[0]
	for _, v := range masterPlaylist.Variants {
		if v.Bandwidth < variant.Bandwidth {
			variant = v
		}
	}
	return variant
}

func downloadAndHandleSegment(baseURL *url.URL, segment *m3u8.MediaSegment, headers map[string]string) error {
	segmentURL, err := baseURL.Parse(segment.URI)
	if err != nil {
		return fmt.Errorf("Error parsing segment %d URL %s: %v", segment.SeqId, segment.URI, err)
	}

	resp, err := makeHTTPRequest("GET", segmentURL.String(), headers)
	if err != nil {
		return fmt.Errorf("Error making request to download segment: %v", err)
	}
	defer resp.Body.Close()

	return handleSegment(context.Background(), segment, resp.Body)
}

func handleSegment(ctx context.Context, segment *m3u8.MediaSegment, body io.ReadCloser) error {
	defer body.Close()
	bytes, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	h := sha256.Sum256(bytes)
	fmt.Printf("%s - segment %d (size: %d) hash: %s\n", time.Now().Format(time.RFC3339Nano), segment.SeqId, len(bytes), hex.EncodeToString(h[:]))
	return nil
}
