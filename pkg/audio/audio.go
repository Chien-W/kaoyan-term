package audio

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Player struct {
	cacheDir string
	muted    bool
	accent   int // 1: UK, 2: US
	mu       sync.Mutex
	client   *http.Client
}

func NewPlayer(accent int) *Player {
	home, _ := os.UserHomeDir()
	cache := filepath.Join(home, ".kaoyan-term", "audio")
	os.MkdirAll(cache, 0755)

	if accent != 1 && accent != 2 {
		accent = 2 // default to US accent
	}

	return &Player{
		cacheDir: cache,
		accent:   accent,
		client:   &http.Client{Timeout: 3 * time.Second},
	}
}

func (p *Player) ToggleMute() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.muted = !p.muted
	return p.muted
}

func (p *Player) IsMuted() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.muted
}

func (p *Player) SetAccent(accent int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if accent == 1 || accent == 2 {
		p.accent = accent
	}
}

// PlayAsync plays audio asynchronously in background
func (p *Player) PlayAsync(word string) {
	go p.Play(word)
}

// Play plays audio for the word
func (p *Player) Play(word string) {
	if p.IsMuted() {
		return
	}

	word = strings.TrimSpace(word)
	if word == "" {
		return
	}

	// Try cached file
	cleanWord := strings.ToLower(word)
	fileName := fmt.Sprintf("%s_t%d.mp3", cleanWord, p.accent)
	filePath := filepath.Join(p.cacheDir, fileName)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// Download from Youdao
		apiURL := fmt.Sprintf("https://dict.youdao.com/dictvoice?audio=%s&type=%d", url.QueryEscape(cleanWord), p.accent)
		resp, err := p.client.Get(apiURL)
		if err == nil && resp.StatusCode == http.StatusOK {
			defer resp.Body.Close()
			f, err := os.Create(filePath)
			if err == nil {
				io.Copy(f, resp.Body)
				f.Close()
			}
		}
	}

	// If file exists, play with afplay (macOS native player)
	if _, err := os.Stat(filePath); err == nil {
		cmd := exec.Command("afplay", filePath)
		cmd.Run()
		return
	}

	// Fallback to macOS system TTS
	cmd := exec.Command("say", "-v", "Samantha", word)
	cmd.Run()
}
