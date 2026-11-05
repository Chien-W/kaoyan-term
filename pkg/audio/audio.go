package audio

import (
	"context"
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
	cancel   context.CancelFunc
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
	if p.muted && p.cancel != nil {
		p.cancel()
	}
	return p.muted
}

func (p *Player) IsMuted() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.muted
}

// Stop terminates any currently active audio playback immediately
func (p *Player) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
}

// PlayAsync plays English pronunciation and then Chinese definition
func (p *Player) PlayAsync(word, definition string) {
	p.mu.Lock()
	if p.cancel != nil {
		p.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.mu.Unlock()

	go p.runPlay(ctx, word, definition)
}

func (p *Player) runPlay(ctx context.Context, word, definition string) {
	if p.IsMuted() {
		return
	}

	word = strings.TrimSpace(word)
	if word == "" {
		return
	}

	// 1. Play English audio
	cleanWord := strings.ToLower(word)
	fileName := fmt.Sprintf("%s_t%d.mp3", cleanWord, p.accent)
	filePath := filepath.Join(p.cacheDir, fileName)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
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

	select {
	case <-ctx.Done():
		return
	default:
	}

	// Play English MP3 using afplay with context cancellation
	if _, err := os.Stat(filePath); err == nil {
		cmd := exec.CommandContext(ctx, "afplay", filePath)
		_ = cmd.Run()
	} else {
		cmd := exec.CommandContext(ctx, "say", "-v", "Samantha", word)
		_ = cmd.Run()
	}

	select {
	case <-ctx.Done():
		return
	default:
	}

	// Short pause between English and Chinese
	time.Sleep(150 * time.Millisecond)

	// 2. Play Chinese definition in headphones (Tingting voice)
	shortDef := extractShortChinese(definition)
	if shortDef != "" {
		cmdCn := exec.CommandContext(ctx, "say", "-v", "Tingting", shortDef)
		_ = cmdCn.Run()
	}
}

func extractShortChinese(def string) string {
	def = strings.TrimSpace(def)
	if def == "" {
		return ""
	}

	// Remove leading part of speech like "v.", "n.", "adj.", "pron."
	for _, pos := range []string{"v.", "n.", "adj.", "adv.", "prep.", "conj.", "pron.", "art.", "num.", "vt.", "vi."} {
		if strings.HasPrefix(def, pos) {
			def = strings.TrimPrefix(def, pos)
			break
		}
	}

	// Clean out newlines
	def = strings.ReplaceAll(def, "\n", " ")

	// Take first 1 or 2 clauses
	parts := strings.FieldsFunc(def, func(r rune) bool {
		return r == '；' || r == ';' || r == '，' || r == ','
	})

	if len(parts) == 0 {
		return ""
	}

	res := strings.TrimSpace(parts[0])
	if len(parts) > 1 && len(res) < 6 {
		res += "，" + strings.TrimSpace(parts[1])
	}

	// Limit length
	runes := []rune(res)
	if len(runes) > 10 {
		res = string(runes[:10])
	}
	return res
}
