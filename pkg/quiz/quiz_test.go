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

func TestAnswerDistribution(t *testing.T) {
	bank, err := vocab.LoadBank()
	if err != nil {
		t.Fatalf("failed to load bank: %v", err)
	}
	engine := NewEngine(bank)
	counts := make(map[string]int)
	var seq []string
	for i := 0; i < 30; i++ {
		w := bank.Words[i+200]
		q := engine.GenerateQuestion(w)
		lbl := q.Options[q.CorrectIndex].Label
		counts[lbl]++
		seq = append(seq, lbl)
	}
	t.Logf("Sequence of answers: %v", seq)
	t.Logf("Counts: %+v", counts)
}
