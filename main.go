package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"kaoyan-term/pkg/audio"
	"kaoyan-term/pkg/state"
	"kaoyan-term/pkg/vocab"
)

var (
	flagStart     = flag.Int("start", -1, "start from specific word index (0-6356)")
	flagWrongOnly = flag.Bool("wrong", false, "practice error log tokens only")
	flagMode      = flag.String("mode", "card", "display mode: card (Baicizhan rich view) or test (Go test disguise)")
	flagStealth   = flag.Bool("stealth", false, "start in stealth microservice log mode")
	flagStatus    = flag.Bool("status", false, "print current study progress dashboard and exit")
	flagReview    = flag.Bool("review", false, "practice today's due spaced repetition reviews only")
)

const (
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
	colorCyan   = "\033[36m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorPurple = "\033[35m"
	colorGray   = "\033[90m"
	colorRed    = "\033[31m"
)

type ViewMode int

const (
	ModeCard ViewMode = iota // 百词斩精美极客卡片
	ModeTest                 // Go Test 单元测试伪装
	ModeLog                  // 极简微服务日志流
)

var ambientLogs = []string{
	"cluster/raft.go:184\tcommit entry index=%d term=14 node=10.200.25.165",
	"storage/wal.go:92\tsync wal segment flushed_bytes=16384 us=182",
	"telemetry/metric.go:142\thistogram bucket recorded metric=\"net_ingress\" p99_ms=1.4",
	"network/pool.go:68\treused idle tcp socket fd=%d remote=\"10.200.25.165:31805\"",
	"pipeline/worker.go:119\tpartition rebalance stable assigned=[0,1,2,3]",
	"cache/redis.go:54\tkey buffer evicted namespace=\"alarm\" freed=128",
}

type StudyItem struct {
	Word     vocab.Word
	IsReview bool
}

func main() {
	flag.Parse()

	bank, err := vocab.LoadBank()
	if err != nil {
		fmt.Fprintf(os.Stderr, "init failed: %v\n", err)
		os.Exit(1)
	}

	prog := state.Load()

	if *flagStatus {
		prog.PrintDashboard(len(bank.Words))
		return
	}

	if *flagStart >= 0 && *flagStart < len(bank.Words) {
		prog.CurrentIndex = *flagStart
	}

	currentMode := ModeCard
	if *flagMode == "test" {
		currentMode = ModeTest
	} else if *flagMode == "log" || *flagStealth {
		currentMode = ModeLog
	}

	player := audio.NewPlayer(2) // 2: US Accent

	wordMap := make(map[string]vocab.Word, len(bank.Words))
	for _, w := range bank.Words {
		wordMap[strings.ToLower(w.Word)] = w
	}

	// Build study queue
	var queue []StudyItem
	if *flagWrongOnly {
		for _, w := range bank.Words {
			if count, ok := prog.WrongWords[w.Word]; ok && count > 0 {
				queue = append(queue, StudyItem{Word: w, IsReview: true})
			}
		}
		if len(queue) == 0 {
			fmt.Println("error book is empty.")
			return
		}
	} else if *flagReview {
		dueWords := prog.GetDueReviewWords()
		for _, dw := range dueWords {
			if w, ok := wordMap[strings.ToLower(dw)]; ok {
				if !prog.IsMastered(w.Word) {
					queue = append(queue, StudyItem{Word: w, IsReview: true})
				}
			}
		}
		if len(queue) == 0 {
			fmt.Println("today's review queue is all clear! Great job.")
			return
		}
	} else {
		// Mixed Spaced Repetition Mode: Due reviews first
		dueWords := prog.GetDueReviewWords()
		for _, dw := range dueWords {
			if w, ok := wordMap[strings.ToLower(dw)]; ok {
				if !prog.IsMastered(w.Word) {
					queue = append(queue, StudyItem{Word: w, IsReview: true})
				}
			}
		}
		// Then new words starting from CurrentIndex
		for i := prog.CurrentIndex; i < len(bank.Words); i++ {
			w := bank.Words[i]
			if !prog.IsMastered(w.Word) {
				queue = append(queue, StudyItem{Word: w, IsReview: false})
			}
		}
	}

	if len(queue) == 0 {
		fmt.Println("all tasks completed.")
		return
	}

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to init raw terminal: %v\n", err)
		return
	}

	restoreTerminal := func(clear bool) {
		player.Stop()
		_ = term.Restore(int(os.Stdin.Fd()), oldState)
		if clear {
			fmt.Print("\033[H\033[2J\033[3J")
		}
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		restoreTerminal(true)
		prog.Save()
		os.Exit(0)
	}()

	defer func() {
		restoreTerminal(false)
		prog.Save()
	}()

	// Initial wipe
	fmt.Print("\033[H\033[2J\033[3J")

	buf := make([]byte, 64)
	nonce := 9840 + prog.CurrentIndex
	queueIndex := 0

WordLoop:
	for {
		if queueIndex >= len(queue) {
			restoreTerminal(false)
			fmt.Println("\r\nservice stopped: all scheduled tasks completed.")
			return
		}

		item := queue[queueIndex]
		target := strings.ToLower(strings.TrimSpace(item.Word.Word))
		if target == "" || prog.IsMastered(target) {
			queueIndex++
			continue
		}

		// Play English audio + Chinese definition in headphones
		player.PlayAsync(target, item.Word.Definition)

		input := ""
		nonce++

		// Render word layout
		renderWordHeader(currentMode, nonce, prog.CurrentIndex+1, len(bank.Words), item, prog)
		renderInputPrompt(currentMode, nonce, prog.CurrentIndex+1, len(bank.Words), item, input, target)

		startTime := time.Now()
		wordError := false

		for {
			n, err := os.Stdin.Read(buf)
			if err != nil || n == 0 {
				continue
			}

			for i := 0; i < n; i++ {
				b := buf[i]

				// Esc (27) or Ctrl+C (3): Boss Key instant clear and exit
				if b == 27 || b == 3 {
					restoreTerminal(true)
					prog.Save()
					return
				}

				// Space (32): Replay bilingual pronunciation
				if b == 32 {
					player.PlayAsync(target, item.Word.Definition)
					continue
				}

				// Tab (9) or '?': Hint next letter
				if b == 9 || b == '?' {
					if len(input) < len(target) {
						nextChar := string(target[len(input)])
						input += nextChar
						renderInputPrompt(currentMode, nonce, prog.CurrentIndex+1, len(bank.Words), item, input, target)
					}
					continue
				}

				// Ctrl+K (11): Permanently kill simple word (秒杀简单词)
				if b == 11 {
					prog.MarkMastered(target)
					prog.Save()
					flashNotice(currentMode, fmt.Sprintf("已秒杀简单词 [%s]，永久移出考研大纲复习流！", target))
					queueIndex++
					continue WordLoop
				}

				// Ctrl+T (20): Toggle View Mode (Card View <-> Go Test Disguise)
				if b == 20 {
					if currentMode == ModeCard {
						currentMode = ModeTest
					} else {
						currentMode = ModeCard
					}
					fmt.Print("\033[H\033[2J\033[3J")
					renderWordHeader(currentMode, nonce, prog.CurrentIndex+1, len(bank.Words), item, prog)
					renderInputPrompt(currentMode, nonce, prog.CurrentIndex+1, len(bank.Words), item, input, target)
					continue
				}

				// Ctrl+V (22): Toggle Stealth Mode (Rich View <-> Stealth Microservice Log)
				if b == 22 {
					if currentMode == ModeLog {
						currentMode = ModeCard
					} else {
						currentMode = ModeLog
					}
					fmt.Print("\033[H\033[2J\033[3J")
					renderWordHeader(currentMode, nonce, prog.CurrentIndex+1, len(bank.Words), item, prog)
					renderInputPrompt(currentMode, nonce, prog.CurrentIndex+1, len(bank.Words), item, input, target)
					continue
				}

				// Ctrl+P (16): Telemetry log
				if b == 16 {
					ts := time.Now().Format("15:04:05.000")
					fmt.Print("\033[2K\r")
					fmt.Printf("%s%s\t%sINFO%s\ttelemetry.go:42\t[2027-NETEM] idx=%d/%d done_today=%d mastered=%d due_rev=%d\r\n",
						colorGray, ts, colorBlue, colorReset, prog.CurrentIndex+1, len(bank.Words), prog.DailyNewCount, len(prog.Records), len(prog.GetDueReviewWords()))
					renderInputPrompt(currentMode, nonce, prog.CurrentIndex+1, len(bank.Words), item, input, target)
					continue
				}

				// Backspace (127 or 8)
				if b == 127 || b == 8 {
					if len(input) > 0 {
						input = input[:len(input)-1]
						renderInputPrompt(currentMode, nonce, prog.CurrentIndex+1, len(bank.Words), item, input, target)
					}
					continue
				}

				// Enter (13 or 10): Submit input token
				if b == 13 || b == 10 {
					trimmedInput := strings.TrimSpace(input)
					if trimmedInput == target {
						goto WordCompleted
					} else {
						// Mismatch: show warning and let user retry
						wordError = true
						showInputMismatch(currentMode, nonce, trimmedInput, len(target))
						player.PlayAsync(target, item.Word.Definition)
						input = ""
						time.Sleep(250 * time.Millisecond)
						renderInputPrompt(currentMode, nonce, prog.CurrentIndex+1, len(bank.Words), item, input, target)
					}
					continue
				}

				// Letter & symbol typing (a-z, A-Z, '-', ' ', '\'')
				if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '-' || b == ' ' || b == '\'' {
					charTyped := strings.ToLower(string(b))
					input += charTyped
					renderInputPrompt(currentMode, nonce, prog.CurrentIndex+1, len(bank.Words), item, input, target)
				}
			}
		}

	WordCompleted:
		elapsed := time.Since(startTime)
		if wordError {
			prog.RecordError(target)
		} else {
			prog.RecordSuccess(target, item.IsReview)
			if !item.IsReview && prog.CurrentIndex < len(bank.Words)-1 {
				prog.CurrentIndex++
			}
		}

		ms := elapsed.Milliseconds()
		if ms == 0 {
			ms = 14 + int64(rand.Intn(35))
		}

		showCompletionPass(currentMode, nonce, target, item.Word, ms, item.IsReview)

		time.Sleep(120 * time.Millisecond)
		queueIndex++
		prog.Save()
	}
}

func renderWordHeader(mode ViewMode, nonce, current, total int, item StudyItem, prog *state.StudyProgress) {
	w := item.Word
	phonetic := w.USPhonetic
	if phonetic == "" {
		phonetic = w.UKPhonetic
	}
	progPct := float64(current) / float64(total) * 100

	switch mode {
	case ModeCard:
		fmt.Print("\033[H\033[2J") // Clear screen
		W := 70
		border := strings.Repeat("─", W-2)

		fmt.Printf("%s┌%s┐%s\r\n", colorCyan, border, colorReset)

		// Header
		typeTag := "新学"
		if item.IsReview {
			typeTag = "复习"
		}
		header := fmt.Sprintf(" [NETEM 2027 考研核心词汇] · [%d/%d] %.1f%% · 今日%s: %d",
			current, total, progPct, typeTag, prog.DailyNewCount)
		fmt.Printf("%s│%s %-66s%s│%s\r\n", colorCyan, colorBold, header, colorCyan, colorReset)
		fmt.Printf("%s├%s┤%s\r\n", colorCyan, border, colorReset)

		// Target Word & Phonetic
		phoStr := ""
		if phonetic != "" {
			phoStr = fmt.Sprintf("/%s/", phonetic)
		}
		fmt.Printf("%s│%s  %s%-24s%s %s%-38s%s%s│%s\r\n",
			colorCyan, colorReset, colorBold, w.Word, colorReset, colorYellow, phoStr, colorReset, colorCyan, colorReset)
		fmt.Printf("%s│%s                                                                  %s│%s\r\n", colorCyan, colorReset, colorCyan, colorReset)

		// POS Items
		fmt.Printf("%s│%s  %s【大纲词性释义】%s                                                    %s│%s\r\n",
			colorCyan, colorReset, colorBlue, colorReset, colorCyan, colorReset)

		if len(w.POSItems) > 0 {
			for _, p := range w.POSItems[:min(4, len(w.POSItems))] {
				t := p.Type
				if t == "" {
					t = "•"
				}
				line := fmt.Sprintf("    %-6s %s", t, p.Translation)
				line = truncateToVisualWidth(line, 64)
				fmt.Printf("%s│%s  %-64s%s│%s\r\n", colorCyan, colorReset, line, colorCyan, colorReset)
			}
		} else {
			defLine := fmt.Sprintf("    %s", cleanDefinition(w.Definition, 60))
			fmt.Printf("%s│%s  %-64s%s│%s\r\n", colorCyan, colorReset, defLine, colorCyan, colorReset)
		}

		// Example Sentence
		if w.Example != "" {
			fmt.Printf("%s│%s                                                                  %s│%s\r\n", colorCyan, colorReset, colorCyan, colorReset)
			fmt.Printf("%s│%s  %s【真题/语境例句】%s                                                    %s│%s\r\n",
				colorCyan, colorReset, colorGreen, colorReset, colorCyan, colorReset)

			exLine := fmt.Sprintf("    en: %s", w.Example)
			exLine = truncateToVisualWidth(exLine, 64)
			fmt.Printf("%s│%s  %-64s%s│%s\r\n", colorCyan, colorReset, exLine, colorCyan, colorReset)

			if w.ExampleTrans != "" {
				zhLine := fmt.Sprintf("    zh: %s", w.ExampleTrans)
				zhLine = truncateToVisualWidth(zhLine, 64)
				fmt.Printf("%s│%s  %-64s%s│%s\r\n", colorCyan, colorReset, zhLine, colorCyan, colorReset)
			}
		}

		// Collocations / Phrases
		if len(w.Phrases) > 0 {
			fmt.Printf("%s│%s                                                                  %s│%s\r\n", colorCyan, colorReset, colorCyan, colorReset)
			fmt.Printf("%s│%s  %s【考点搭配】%s                                                        %s│%s\r\n",
				colorCyan, colorReset, colorPurple, colorReset, colorCyan, colorReset)
			for _, phr := range w.Phrases[:min(2, len(w.Phrases))] {
				phrLine := fmt.Sprintf("    • %s", phr)
				phrLine = truncateToVisualWidth(phrLine, 64)
				fmt.Printf("%s│%s  %-64s%s│%s\r\n", colorCyan, colorReset, phrLine, colorCyan, colorReset)
			}
		}

		fmt.Printf("%s└%s┘%s\r\n", colorCyan, border, colorReset)
		fmt.Printf("%s  [空格] 发音 | [Tab] 提示 | [Ctrl+K] 秒杀 | [Ctrl+T] 伪装 | [Ctrl+V] 隐身 | [Esc] 退出%s\r\n", colorGray, colorReset)

	case ModeTest:
		fmt.Printf("\r\n=== RUN   TestMetricCollector/QueryPoint_%04d [%s] /%s/\r\n",
			current, w.Word, phonetic)

		// Multi-line POS
		if len(w.POSItems) > 0 {
			for idx, p := range w.POSItems[:min(3, len(w.POSItems))] {
				t := p.Type
				if t == "" {
					t = "def"
				}
				fmt.Printf("    collector_test.go:%d: [%s] %s\r\n", 40+idx, t, p.Translation)
			}
		} else {
			fmt.Printf("    collector_test.go:40: [param] %s\r\n", cleanDefinition(w.Definition, 60))
		}

		// Example
		if w.Example != "" {
			fmt.Printf("    collector_test.go:44: [example] %s\r\n", w.Example)
			if w.ExampleTrans != "" {
				fmt.Printf("    collector_test.go:45: [translate] %s\r\n", w.ExampleTrans)
			}
		}

		// Phrase
		if len(w.Phrases) > 0 {
			fmt.Printf("    collector_test.go:46: [phrase] %s\r\n", w.Phrases[0])
		}

	case ModeLog:
		// Do not print header in microservice log stream
	}
}

func renderInputPrompt(mode ViewMode, nonce, current, total int, item StudyItem, input, target string) {
	coloredInput := formatInput(input, target)

	switch mode {
	case ModeCard:
		fmt.Print("\033[2K\r")
		fmt.Printf("  %s键入:%s %s█%s", colorBold, colorReset, coloredInput, colorReset)

	case ModeTest:
		fmt.Print("\033[2K\r")
		fmt.Printf("    collector_test.go:48: [token] %s█%s", coloredInput, colorReset)

	case ModeLog:
		ts := time.Now().Format("15:04:05.000")
		fmt.Print("\033[2K\r")
		fmt.Printf("%s%s\t%sDEBUG%s\trule.go:88\tpayload [%d/%d] len=%d token=%s█%s",
			colorGray, ts, colorYellow, colorReset, current, total, len(target), coloredInput, colorReset)
	}
}

func showInputMismatch(mode ViewMode, nonce int, input string, targetLen int) {
	fmt.Print("\033[2K\r")
	switch mode {
	case ModeCard:
		fmt.Printf("  %s✗ 拼写有误 (%q, 长度%d)，请按退格键修改或按 Tab 提示%s", colorRed, input, targetLen, colorReset)
	case ModeTest:
		fmt.Printf("    collector_test.go:49: %s[mismatch] input=%q len=%d retry%s", colorRed, input, targetLen, colorReset)
	case ModeLog:
		ts := time.Now().Format("15:04:05.000")
		fmt.Printf("%s%s\t%sWARN%s\tauth.go:68\ttoken mismatch input=%q target_len=%d status=401 retry\r\n",
			colorGray, ts, colorRed, colorReset, input, targetLen)
	}
}

func showCompletionPass(mode ViewMode, nonce int, target string, w vocab.Word, ms int64, isReview bool) {
	fmt.Print("\033[2K\r")
	ts := time.Now().Format("15:04:05.000")

	switch mode {
	case ModeCard:
		tag := "新词掌握"
		if isReview {
			tag = "复习巩固"
		}
		fmt.Printf("  %s✔ 完美掌握 [%s] (%s, %dms)%s\r\n", colorGreen, target, tag, ms, colorReset)

	case ModeTest:
		fmt.Printf("    collector_test.go:50: [pass] verified token=%q ms=%d\r\n", target, ms)
		fmt.Printf("--- PASS: TestMetricCollector/QueryPoint_%04d (%.2fs)\r\n", nonce%1000, float64(ms)/1000.0)

	case ModeLog:
		fmt.Printf("%s%s\t%sINFO%s\tauth.go:72\tsession handshake verified token=%q status=200 ms=%d\r\n",
			colorGray, ts, colorBlue, colorReset, target, ms)
		if rand.Float32() < 0.35 {
			pattern := ambientLogs[rand.Intn(len(ambientLogs))]
			logMsg := fmt.Sprintf(pattern, rand.Intn(50000))
			hbTs := time.Now().Format("15:04:05.000")
			fmt.Printf("%s%s\t%sINFO%s\t%s\r\n", colorGray, hbTs, colorBlue, colorReset, logMsg)
		}
	}
}

func flashNotice(mode ViewMode, msg string) {
	fmt.Print("\033[2K\r")
	fmt.Printf("%s  ⚡ %s%s\r\n", colorYellow, msg, colorReset)
	time.Sleep(300 * time.Millisecond)
}

func formatInput(input, target string) string {
	if input == "" {
		return ""
	}
	var b strings.Builder
	for i := 0; i < len(input); i++ {
		ch := input[i]
		if i < len(target) && ch == target[i] {
			b.WriteString(colorCyan)
			b.WriteByte(ch)
			b.WriteString(colorReset)
		} else {
			b.WriteString(colorRed)
			b.WriteByte(ch)
			b.WriteString(colorReset)
		}
	}
	return b.String()
}

func cleanDefinition(raw string, maxLen int) string {
	raw = strings.ReplaceAll(raw, "\n", " ")
	runes := []rune(raw)
	if len(runes) > maxLen {
		return string(runes[:maxLen-2]) + ".."
	}
	return raw
}

func truncateToVisualWidth(s string, maxWidth int) string {
	runes := []rune(s)
	width := 0
	cutIdx := len(runes)

	for i, r := range runes {
		if r > 127 {
			width += 2
		} else {
			width += 1
		}
		if width > maxWidth-2 {
			cutIdx = i
			break
		}
	}

	if cutIdx < len(runes) {
		return string(runes[:cutIdx]) + ".."
	}
	return s
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
