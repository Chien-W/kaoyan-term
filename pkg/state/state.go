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
	Stage       int       `json:"stage"` // 0: new, 1: 1d, 2: 2d, 3: 4d, 4: 7d, 5: 15d, 6: mastered
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
	return os.WriteFile(stateFilePath(), data, 0644)
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

	// Remove from wrong words if successful
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
	rec.Stage = 1                                // Reset to stage 1 (review tomorrow)
	rec.NextReview = now.Add(12 * time.Hour)     // Due soon
	p.WrongWords[word]++
}

// GetDueReviewWords returns words that have reached their review interval
func (p *StudyProgress) GetDueReviewWords() []string {
	now := time.Now()
	var due []string
	for word, rec := range p.Records {
		if !rec.NextReview.IsZero() && now.After(rec.NextReview) {
			due = append(due, word)
		}
	}
	return due
}

// PrintDashboard prints study stats in plain text
func (p *StudyProgress) PrintDashboard(totalWords int) {
	learnedCount := len(p.Records)
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
	fmt.Printf("• 待学剩余词汇:   %d 词\n", remaining)
	fmt.Printf("• 当前大纲指针:   第 %d 词\n", p.CurrentIndex+1)
	fmt.Println("--------------------------------------------------")
	fmt.Println(" 📅 今日任务 (Daily Goals):")
	fmt.Printf("• 今日新学完成:   %d / %d 词\n", p.DailyNewCount, p.DailyGoal)
	fmt.Printf("• 今日复习完成:   %d 词\n", p.DailyRevCount)
	fmt.Printf("• 今日到期待复习: %d 词 (基于艾宾浩斯遗忘曲线)\n", dueCount)
	fmt.Printf("• 错题待重难巩固: %d 词\n", wrongCount)
	fmt.Println("==================================================")
	fmt.Printf("💡 建议：距离 2027 考研约 415 天，按每天新学 30 词计算，约 7 个月可完整过完 3 轮大纲词！\n\n")
}
