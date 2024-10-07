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

	// Create a new request
	req, err := http.NewRequest("GET", masterManifestURL.String(), nil)
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return
	}

	// Add headers to the request
	for key, value := range headers {
		req.Header.Add(key, value)
	}

	// Perform the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error making request:", err)
		return
	}
	defer resp.Body.Close()

	// parse the master manifest
	playlist, listType, err := m3u8.DecodeFrom(resp.Body, true)
	if err != nil {
		fmt.Println("Error parsing manifest:", err)
		return
	}
	if listType != m3u8.MASTER {
		fmt.Println("Not a master playlist")
		return
	}

	// no variant streams
	masterPlaylist := playlist.(*m3u8.MasterPlaylist)
	if len(masterPlaylist.Variants) == 0 {
		fmt.Println("No variant streams")
		return
	}

	// pick the minimal bandwidth variant
	variant := masterPlaylist.Variants[0]
	for _, v := range masterPlaylist.Variants {
		if v.Bandwidth < variant.Bandwidth {
			variant = v
		}
	}

	mediaPlaylistURL, err := masterManifestURL.Parse(variant.URI)
	if err != nil {
		fmt.Println("Error parsing media playlist URL:", err)
		return
	}

	// Get the variant stream
	req, err = http.NewRequest("GET", mediaPlaylistURL.String(), nil)
	if err != nil {
		fmt.Println("Error creating media playlist request:", err)
		return
	}

	// Add headers to the request
	for key, value := range headers {
		req.Header.Add(key, value)
	}

	// Perform the request
	resp, err = client.Do(req)
	if err != nil {
		fmt.Println("Error making request to download media playlist:", err)
		return
	}
	defer resp.Body.Close()

	// parse the media playlist
	playlist, listType, err = m3u8.DecodeFrom(resp.Body, true)
	if err != nil {
		fmt.Println("Error parsing media playlist:", err)
		return
	}

	if listType != m3u8.MEDIA {
		fmt.Println("Not a media playlist")
		return
	}

	mediaPlaylist := playlist.(*m3u8.MediaPlaylist)
	fmt.Printf("mediaPlaylist: %v\n", mediaPlaylist)

	// download the segments and handle them
	segments := mediaPlaylist.Segments
	for _, segment := range segments {
		segmentURL, err := mediaPlaylistURL.Parse(segment.URI)
		if err != nil {
			fmt.Println("Error parsing segment %d URL %s: %v", segment.SeqId, segment.URI, err)
			return
		}
		// download the segment
		req, err = http.NewRequest("GET", segmentURL.String(), nil)
		if err != nil {
			fmt.Println("Error creating segment request:", err)
			return
		}

		// Add headers to the request
		for key, value := range headers {
			req.Header.Add(key, value)
		}

		// Perform the request
		resp, err = client.Do(req)
		if err != nil {
			fmt.Println("Error making request to download segment:", err)
			return
		}

		// handle the segment
		err = handleSegment(context.Background(), segment, resp.Body)
		if err != nil {
			fmt.Println("Error handling segment:", err)
			return
		}

	}
}

func handleSegment(background context.Context, segment *m3u8.MediaSegment, body io.ReadCloser) error {
	defer body.Close()
	bytes, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	h := sha256.Sum256(bytes)
	fmt.Printf("%s - segment %d (size: %d) hash: %s\n", time.Now().Format(time.RFC3339Nano), segment.SeqId, len(bytes), hex.EncodeToString(h[:]))
	return nil
}
