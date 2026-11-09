package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type WordRecord struct {
	Word        string    `json:"word"`
	ReviewCount int       `json:"review_count"`
	ErrorCount  int       `json:"error_count"`
	LastSeen    time.Time `json:"last_seen"`
	NextReview  time.Time `json:"next_review"`
	Stage       int       `json:"stage"` // 0: new, 1: 1d, 2: 2d, 3: 4d, 4: 7d, 5: 15d, 6: 30d, 99: permanently mastered
}

type StudyProgress struct {
	CurrentIndex  int                    `json:"current_index"`
	Records       map[string]*WordRecord `json:"records"`
	WrongWords    map[string]int         `json:"wrong_words"`
	DailyDate     string                 `json:"daily_date"`
	DailyNewCount int                    `json:"daily_new_count"`
	DailyRevCount int                    `json:"daily_rev_count"`
	DailyGoal     int                    `json:"daily_goal"` // default 50
}

func stateFilePath() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".kaoyan-term")
	os.MkdirAll(dir, 0755)
	return filepath.Join(dir, "state.json")
}

func Load() *StudyProgress {
	p := &StudyProgress{
		Records:    make(map[string]*WordRecord),
		WrongWords: make(map[string]int),
		DailyGoal:  50,
	}
	today := time.Now().Format("2006-01-02")

	data, err := os.ReadFile(stateFilePath())
	if err == nil {
		_ = json.Unmarshal(data, p)
	}

	if p.Records == nil {
		p.Records = make(map[string]*WordRecord)
	}
	if p.WrongWords == nil {
		p.WrongWords = make(map[string]int)
	}
	if p.DailyGoal <= 0 {
		p.DailyGoal = 50
	}
	if p.DailyDate != today {
		p.DailyDate = today
		p.DailyNewCount = 0
		p.DailyRevCount = 0
	}

	return p
}

func (p *StudyProgress) Save() error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	targetPath := stateFilePath()
	tmpPath := fmt.Sprintf("%s.%d.tmp", targetPath, time.Now().UnixNano())
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, targetPath)
}

// MarkMastered permanently kills a simple word (斩词/秒杀)
func (p *StudyProgress) MarkMastered(word string) {
	now := time.Now()
	rec, exists := p.Records[word]
	if !exists {
		rec = &WordRecord{
			Word: word,
		}
		p.Records[word] = rec
	}
	rec.LastSeen = now
	rec.Stage = 99                                        // 99 = permanent mastered
	rec.NextReview = now.Add(3650 * 24 * time.Hour)       // 10 years later
	delete(p.WrongWords, word)
}

func (p *StudyProgress) IsMastered(word string) bool {
	if rec, ok := p.Records[word]; ok {
		return rec.Stage >= 99
	}
	return false
}

// RecordSuccess updates spaced repetition curve
func (p *StudyProgress) RecordSuccess(word string, isReview bool) {
	now := time.Now()
	rec, exists := p.Records[word]
	if !exists {
		rec = &WordRecord{
			Word:  word,
			Stage: 0,
		}
		p.Records[word] = rec
	}

	rec.LastSeen = now
	rec.ReviewCount++

	if isReview {
		p.DailyRevCount++
	} else {
		p.DailyNewCount++
	}

	// If already permanently mastered, keep it
	if rec.Stage >= 99 {
		return
	}

	// Spaced repetition intervals: 1d -> 2d -> 4d -> 7d -> 15d -> 30d
	intervals := []time.Duration{
		24 * time.Hour,
		48 * time.Hour,
		96 * time.Hour,
		7 * 24 * time.Hour,
		15 * 24 * time.Hour,
		30 * 24 * time.Hour,
	}

	if rec.Stage < len(intervals) {
		rec.NextReview = now.Add(intervals[rec.Stage])
		rec.Stage++
	} else {
		rec.NextReview = now.Add(45 * 24 * time.Hour)
	}

	if count, ok := p.WrongWords[word]; ok {
		if count <= 1 {
			delete(p.WrongWords, word)
		} else {
			p.WrongWords[word] = count - 1
		}
	}
}

// RecordError marks failure and resets to review stage 1
func (p *StudyProgress) RecordError(word string) {
	now := time.Now()
	rec, exists := p.Records[word]
	if !exists {
		rec = &WordRecord{
			Word:  word,
			Stage: 0,
		}
		p.Records[word] = rec
	}

	rec.LastSeen = now
	rec.ErrorCount++
	rec.Stage = 1
	rec.NextReview = now.Add(12 * time.Hour)
	p.WrongWords[word]++
}

// GetDueReviewWords returns words that have reached their review interval (excluding permanently mastered)
func (p *StudyProgress) GetDueReviewWords() []string {
	now := time.Now()
	var due []string
	for word, rec := range p.Records {
		if rec.Stage < 99 && !rec.NextReview.IsZero() && now.After(rec.NextReview) {
			due = append(due, word)
		}
	}
	return due
}

// PrintDashboard prints study stats in plain text
func (p *StudyProgress) PrintDashboard(totalWords int) {
	learnedCount := len(p.Records)
	masteredCount := 0
	for _, rec := range p.Records {
		if rec.Stage >= 99 {
			masteredCount++
		}
	}

	remaining := totalWords - learnedCount
	if remaining < 0 {
		remaining = 0
	}
	progressPct := float64(learnedCount) / float64(totalWords) * 100

	dueCount := len(p.GetDueReviewWords())
	wrongCount := len(p.WrongWords)

	fmt.Println("==================================================")
	fmt.Println(" 📚 2027 考研英语大纲词汇复习看板 (NETEM Tracker)")
	fmt.Println("==================================================")
	fmt.Printf("• 词库大纲总量:   %d 词\n", totalWords)
	fmt.Printf("• 累计已学词汇:   %d 词 (完成度: %.1f%%)\n", learnedCount, progressPct)
	fmt.Printf("• 斩词/永久掌握:  %d 词 (标记为完全掌握，永不再考)\n", masteredCount)
	fmt.Printf("• 待学剩余词汇:   %d 词\n", remaining)
	fmt.Printf("• 当前大纲指针:   第 %d 词\n", p.CurrentIndex+1)
	fmt.Println("--------------------------------------------------")
	fmt.Println(" 📅 今日任务 (Daily Goals):")
	fmt.Printf("• 今日新学完成:   %d / %d 词\n", p.DailyNewCount, p.DailyGoal)
	fmt.Printf("• 今日复习完成:   %d 词\n", p.DailyRevCount)
	fmt.Printf("• 今日到期待复习: %d 词 (基于艾宾浩斯遗忘曲线)\n", dueCount)
	fmt.Printf("• 错题待重难巩固: %d 词\n", wrongCount)
	fmt.Println("==================================================")
	fmt.Printf("💡 建议：遇到非常简单的已知词，随时按 [Ctrl+K] 斩词秒杀，永久移出复习流！\n\n")
}
