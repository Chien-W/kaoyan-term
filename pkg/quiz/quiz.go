package quiz

import (
	"math/rand"
	"sort"
	"strings"

	"kaoyan-term/pkg/vocab"
)

type Option struct {
	Label       string // "A", "B", "C", "D"
	Word        string // word spelling (e.g. "contact")
	Definition  string // concise Chinese definition
	IsCorrect   bool
	IsConfusing bool
}

type Question struct {
	TargetWord   vocab.Word
	Options      []Option
	CorrectIndex int // 0, 1, 2, 3
}

type Engine struct {
	bank     *vocab.Bank
	wordMap  map[string]vocab.Word
	wordList []string
}

func NewEngine(bank *vocab.Bank) *Engine {
	wordMap := make(map[string]vocab.Word, len(bank.Words))
	wordList := make([]string, 0, len(bank.Words))

	for _, w := range bank.Words {
		clean := strings.ToLower(strings.TrimSpace(w.Word))
		if clean != "" {
			wordMap[clean] = w
			wordList = append(wordList, clean)
		}
	}

	return &Engine{
		bank:     bank,
		wordMap:  wordMap,
		wordList: wordList,
	}
}

type candidate struct {
	word  string
	score int
}

func (e *Engine) GenerateQuestion(target vocab.Word) Question {
	tStr := strings.ToLower(strings.TrimSpace(target.Word))
	tLen := len(tStr)

	var candidates []candidate

	// Find confusing words by spelling similarity and prefix
	for _, w := range e.wordList {
		if w == tStr {
			continue
		}
		wLen := len(w)
		diffLen := tLen - wLen
		if diffLen < 0 {
			diffLen = -diffLen
		}
		if diffLen > 2 {
			continue
		}

		dist := levenshtein(tStr, w)
		if dist <= 3 {
			// Calculate common prefix
			cp := 0
			minL := tLen
			if wLen < minL {
				minL = wLen
			}
			for cp < minL && tStr[cp] == w[cp] {
				cp++
			}

			// Calculate common suffix
			cs := 0
			for cs < minL && tStr[tLen-1-cs] == w[wLen-1-cs] {
				cs++
			}

			// Lower score = more confusing (closer match)
			score := dist*10 - cp*3 - cs*2
			candidates = append(candidates, candidate{word: w, score: score})
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score < candidates[j].score
	})

	distractorWords := make([]string, 0, 3)
	for _, c := range candidates {
		if len(distractorWords) >= 3 {
			break
		}
		// Avoid duplicates
		distractorWords = append(distractorWords, c.word)
	}

	// Fallback if fewer than 3 confusing candidates
	for len(distractorWords) < 3 {
		rw := e.wordList[rand.Intn(len(e.wordList))]
		if rw != tStr && !contains(distractorWords, rw) {
			distractorWords = append(distractorWords, rw)
		}
	}

	type rawOpt struct {
		word        string
		isCorrect   bool
		isConfusing bool
	}

	rawOptions := []rawOpt{
		{word: tStr, isCorrect: true, isConfusing: false},
		{word: distractorWords[0], isCorrect: false, isConfusing: true},
		{word: distractorWords[1], isCorrect: false, isConfusing: true},
		{word: distractorWords[2], isCorrect: false, isConfusing: true},
	}

	// Shuffle options
	rand.Shuffle(len(rawOptions), func(i, j int) {
		rawOptions[i], rawOptions[j] = rawOptions[j], rawOptions[i]
	})

	labels := []string{"A", "B", "C", "D"}
	options := make([]Option, 4)
	correctIdx := 0

	for i, ro := range rawOptions {
		wItem, ok := e.wordMap[ro.word]
		def := ""
		if ok {
			def = formatOptionDefinition(wItem)
		} else {
			def = ro.word
		}

		if ro.isCorrect {
			correctIdx = i
		}

		options[i] = Option{
			Label:       labels[i],
			Word:        ro.word,
			Definition:  def,
			IsCorrect:   ro.isCorrect,
			IsConfusing: ro.isConfusing,
		}
	}

	return Question{
		TargetWord:   target,
		Options:      options,
		CorrectIndex: correctIdx,
	}
}

func formatOptionDefinition(w vocab.Word) string {
	if len(w.POSItems) > 0 {
		var parts []string
		for _, p := range w.POSItems[:min(2, len(w.POSItems))] {
			t := p.Type
			if t != "" && !strings.HasSuffix(t, ".") {
				t += "."
			}
			parts = append(parts, strings.TrimSpace(t+" "+p.Translation))
		}
		res := strings.Join(parts, "；")
		return truncateRunes(res, 38)
	}

	// Fallback to definition
	raw := strings.ReplaceAll(w.Definition, "\n", " ")
	return truncateRunes(raw, 38)
}

func truncateRunes(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) > maxLen {
		return string(runes[:maxLen-2]) + ".."
	}
	return s
}

func levenshtein(s1, s2 string) int {
	r1, r2 := []rune(s1), []rune(s2)
	n1, n2 := len(r1), len(r2)
	if n1 == 0 {
		return n2
	}
	if n2 == 0 {
		return n1
	}

	prev := make([]int, n2+1)
	for j := 0; j <= n2; j++ {
		prev[j] = j
	}

	for i := 1; i <= n1; i++ {
		curr := make([]int, n2+1)
		curr[0] = i
		for j := 1; j <= n2; j++ {
			cost := 0
			if r1[i-1] != r2[j-1] {
				cost = 1
			}
			curr[j] = min3(
				curr[j-1]+1,    // insertion
				prev[j]+1,      // deletion
				prev[j-1]+cost, // substitution
			)
		}
		prev = curr
	}
	return prev[n2]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func contains(arr []string, s string) bool {
	for _, item := range arr {
		if item == s {
			return true
		}
	}
	return false
}
