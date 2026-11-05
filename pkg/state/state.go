package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type Progress struct {
	CurrentIndex  int            `json:"current_index"`
	MasteredCount int            `json:"mastered_count"`
	WrongWords    map[string]int `json:"wrong_words"`
	StealthMode   bool           `json:"stealth_mode"`
	DictationMode bool           `json:"dictation_mode"`
	DailyDate     string         `json:"daily_date"`
	DailyCount    int            `json:"daily_count"`
}

func stateFilePath() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".kaoyan-term")
	os.MkdirAll(dir, 0755)
	return filepath.Join(dir, "state.json")
}

func Load() *Progress {
	p := &Progress{
		WrongWords: make(map[string]int),
	}
	today := time.Now().Format("2006-01-02")

	data, err := os.ReadFile(stateFilePath())
	if err == nil {
		_ = json.Unmarshal(data, p)
	}

	if p.WrongWords == nil {
		p.WrongWords = make(map[string]int)
	}
	if p.DailyDate != today {
		p.DailyDate = today
		p.DailyCount = 0
	}

	return p
}

func (p *Progress) Save() error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(stateFilePath(), data, 0644)
}

func (p *Progress) RecordSuccess(word string) {
	p.DailyCount++
	p.MasteredCount++
	if p.CurrentIndex < 6356 {
		p.CurrentIndex++
	}
	// If in wrong words, reduce weight
	if count, ok := p.WrongWords[word]; ok {
		if count <= 1 {
			delete(p.WrongWords, word)
		} else {
			p.WrongWords[word] = count - 1
		}
	}
}

func (p *Progress) RecordError(word string) {
	p.WrongWords[word]++
}
