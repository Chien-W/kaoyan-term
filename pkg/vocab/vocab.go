package vocab

import (
	_ "embed"
	"encoding/json"
	"strings"
)

//go:embed words.json
var embeddedWordsJSON []byte

type Word struct {
	Word        string   `json:"word"`
	USPhonetic  string   `json:"us_phonetic"`
	UKPhonetic  string   `json:"uk_phonetic"`
	Definition  string   `json:"definition"`
	Frequency   float64  `json:"frequency"`
	Phrases     []string `json:"phrases"`
}

type Bank struct {
	Words []Word
}

func LoadBank() (*Bank, error) {
	var words []Word
	err := json.Unmarshal(embeddedWordsJSON, &words)
	if err != nil {
		return nil, err
	}
	return &Bank{Words: words}, nil
}

// FilterByQuery searches words by prefix or definition keyword
func (b *Bank) FilterByQuery(q string) []Word {
	q = strings.ToLower(strings.TrimSpace(q))
	var res []Word
	for _, w := range b.Words {
		if strings.HasPrefix(strings.ToLower(w.Word), q) || strings.Contains(w.Definition, q) {
			res = append(res, w)
		}
	}
	return res
}
