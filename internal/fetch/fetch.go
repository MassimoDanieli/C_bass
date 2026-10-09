// Package fetch brings in a recording from a link: the address of an audio file on the web.
//
// Only a file is fetched. A page that plays music, YouTube's or any other, is not a file, and
// what is in it is not taken out of it: such a link is refused with a reason.
package fetch

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/MassimoDanieli/c_bass/internal/audio"
)

// Limit is the largest recording fetched: a long one in WAV.
const Limit = 600 << 20

// The reasons a link is refused, for the window to explain.
var (
	ErrNotALink   = errors.New("not a link: it should start with http:// or https://")
	ErrVideoSite  = errors.New("a video site gives no audio file: download the recording yourself and drop it on the window")
	ErrNotAudio   = errors.New("the link leads to a page, not to an audio file")
	ErrTooLarge   = fmt.Errorf("the file is larger than %d MB", Limit>>20)
	ErrNotReached = errors.New("the file could not be fetched")
)

// videoSites give pages that play music, never the file itself.
var videoSites = []string{"youtube.com", "youtu.be", "youtube-nocookie.com", "vimeo.com", "tiktok.com", "instagram.com", "facebook.com", "spotify.com", "soundcloud.com", "music.apple.com", "deezer.com"}

// Check says whether a text is a link worth trying, and if not, why.
func Check(text string) (*url.URL, error) {
	link, err := url.Parse(strings.TrimSpace(text))
	if err != nil || (link.Scheme != "http" && link.Scheme != "https") || link.Host == "" {
		return nil, ErrNotALink
	}
	host := strings.ToLower(link.Hostname())
	for _, site := range videoSites {
		if host == site || strings.HasSuffix(host, "."+site) {
			return nil, ErrVideoSite
		}
	}
	return link, nil
}

// kinds are the endings for the types of audio a server names.
var kinds = map[string]string{
	"audio/mpeg": ".mp3", "audio/mp3": ".mp3", "audio/wav": ".wav", "audio/x-wav": ".wav", "audio/wave": ".wav",
	"audio/vnd.wave": ".wav", "audio/flac": ".flac", "audio/x-flac": ".flac", "audio/mp4": ".m4a", "audio/x-m4a": ".m4a",
	"audio/m4a": ".m4a", "audio/aac": ".aac", "audio/ogg": ".ogg", "audio/opus": ".opus", "audio/aiff": ".aiff", "audio/x-aiff": ".aiff",
}

// Get fetches the file a link leads to, telling how far it has got, and gives it with a
// name: the one the server gives, or the last part of the address, with the ending of its kind.
func Get(text string, progress func(done, total int64)) ([]byte, string, error) {
	link, err := Check(text)
	if err != nil {
		return nil, "", err
	}
	client := &http.Client{Timeout: 20 * time.Minute}
	request, err := http.NewRequest(http.MethodGet, link.String(), nil)
	if err != nil {
		return nil, "", ErrNotALink
	}
	request.Header.Set("User-Agent", "C_bass (+https://github.com/MassimoDanieli/C_bass)")
	request.Header.Set("Accept", "audio/*, application/octet-stream;q=0.8, */*;q=0.1")
	response, err := client.Do(request)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrNotReached, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("%w: the server answered %s", ErrNotReached, response.Status)
	}
	if response.ContentLength > Limit {
		return nil, "", ErrTooLarge
	}
	kind, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if strings.HasPrefix(kind, "text/") || kind == "application/xhtml+xml" || kind == "application/json" {
		return nil, "", ErrNotAudio
	}
	name := ""
	if _, params, err := mime.ParseMediaType(response.Header.Get("Content-Disposition")); err == nil {
		name = path.Base(params["filename"])
	}
	if name == "" || name == "." || name == "/" {
		name, _ = url.PathUnescape(path.Base(response.Request.URL.Path)) // after any redirect
	}
	if name == "" || name == "." || name == "/" {
		name = "recording"
	}
	if !audio.Readable(name) {
		if ending, ok := kinds[kind]; ok {
			name += ending
		}
	}
	data, err := read(response.Body, response.ContentLength, progress)
	if err != nil {
		return nil, "", err
	}
	// a page sent without saying so
	if head := strings.ToLower(strings.TrimSpace(string(data[:min(len(data), 512)]))); strings.HasPrefix(head, "<!doctype html") || strings.HasPrefix(head, "<html") {
		return nil, "", ErrNotAudio
	}
	return data, name, nil
}

func read(body io.Reader, total int64, progress func(done, total int64)) ([]byte, error) {
	var data []byte
	buffer := make([]byte, 256<<10)
	for {
		n, err := body.Read(buffer)
		data = append(data, buffer[:n]...)
		if len(data) > Limit {
			return nil, ErrTooLarge
		}
		if progress != nil && n > 0 {
			progress(int64(len(data)), max(total, 0))
		}
		if err == io.EOF {
			return data, nil
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrNotReached, err)
		}
	}
}
