package quiz

import (
	"testing"

	"kaoyan-term/pkg/vocab"
)

func TestEngine(t *testing.T) {
	bank, err := vocab.LoadBank()
	if err != nil {
		t.Fatalf("failed to load bank: %v", err)
	}

	engine := NewEngine(bank)

	testWords := []string{"contract", "adapt", "expand", "statute", "affect"}
	for _, tw := range testWords {
		var target vocab.Word
		for _, w := range bank.Words {
			if w.Word == tw {
				target = w
				break
			}
		}

		q := engine.GenerateQuestion(target)
		if len(q.Options) != 4 {
			t.Errorf("expected 4 options, got %d", len(q.Options))
		}
		if !q.Options[q.CorrectIndex].IsCorrect {
			t.Errorf("correct index %d does not point to correct option", q.CorrectIndex)
		}

		t.Logf("Word: %s (Correct: %s)", tw, q.Options[q.CorrectIndex].Label)
		for _, opt := range q.Options {
			t.Logf("  [%s] %s (%s)", opt.Label, opt.Definition, opt.Word)
		}
	}
}
