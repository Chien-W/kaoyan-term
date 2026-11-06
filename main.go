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
	flagShowDef   = flag.Bool("def", true, "show business expression definition hint in logs")
	flagStatus    = flag.Bool("status", false, "print current study progress dashboard and exit")
	flagReview    = flag.Bool("review", false, "practice today's due spaced repetition reviews only")
)

const (
	colorReset  = "\033[0m"
	colorCyan   = "\033[36m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorGray   = "\033[90m"
	colorRed    = "\033[31m"
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

	// If -status / -stat flag, print visual dashboard and exit cleanly
	if *flagStatus {
		prog.PrintDashboard(len(bank.Words))
		return
	}

	if *flagStart >= 0 && *flagStart < len(bank.Words) {
		prog.CurrentIndex = *flagStart
	}

	showDef := *flagShowDef
	player := audio.NewPlayer(2) // 2: US Accent

	wordMap := make(map[string]vocab.Word, len(bank.Words))
	for _, w := range bank.Words {
		wordMap[strings.ToLower(w.Word)] = w
	}

	// Build queue: Due reviews first, then new words (skipping permanently mastered words)
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
		// Mixed Smart Mode: Due reviews first
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

	// Wipe screen and scrollback completely on start
	fmt.Print("\033[H\033[2J\033[3J")
	printStartupLogs()

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

		// Play English + Chinese definition in headphones
		player.PlayAsync(target, item.Word.Definition)

		input := ""
		nonce++
		renderPromptLine(nonce, prog.CurrentIndex+1, len(bank.Words), item, input, showDef)

		startTime := time.Now()
		wordError := false

		for {
			n, err := os.Stdin.Read(buf)
			if err != nil || n == 0 {
				break
			}

			for i := 0; i < n; i++ {
				b := buf[i]

				// Esc (27) or Ctrl+C (3) -> Boss Key: instant full wipe
				if b == 27 || b == 3 {
					restoreTerminal(true)
					prog.Save()
					return
				}

				// Ctrl+K (11) or Ctrl+E (5) -> Kill word / Mark as fully known (斩词秒杀)
				if b == 11 || b == 5 {
					prog.MarkMastered(target)
					player.Stop()

					ts := time.Now().Format("15:04:05.000")
					fmt.Print("\033[2K\r")
					fmt.Printf("%s%s\t%sINFO%s\tfilter.go:34\tpermanent_bypass target=%q status=skipped\r\n",
						colorGray, ts, colorGreen, colorReset, target)

					if !item.IsReview && prog.CurrentIndex < len(bank.Words)-1 {
						prog.CurrentIndex++
					}
					time.Sleep(80 * time.Millisecond)
					queueIndex++
					prog.Save()
					continue WordLoop
				}

				// Ctrl+V (22) -> Toggle definition visibility and wipe scrollback
				if b == 22 {
					showDef = !showDef
					if !showDef {
						fmt.Print("\033[H\033[2J\033[3J")
						printStartupLogs()
					}
					renderPromptLine(nonce, prog.CurrentIndex+1, len(bank.Words), item, input, showDef)
					continue
				}

				// Ctrl+P (16) -> Print disguised telemetry progress stats in log
				if b == 16 {
					ts := time.Now().Format("15:04:05.000")
					remaining := len(bank.Words) - len(prog.Records)
					if remaining < 0 {
						remaining = 0
					}
					fmt.Print("\033[2K\r")
					fmt.Printf("%s%s\t%sINFO%s\tmetric.go:50\tstats offset=%d/%d left=%d new=%d/%d rev=%d due=%d\r\n",
						colorGray, ts, colorBlue, colorReset,
						prog.CurrentIndex+1, len(bank.Words), remaining,
						prog.DailyNewCount, prog.DailyGoal, prog.DailyRevCount, len(prog.GetDueReviewWords()))
					renderPromptLine(nonce, prog.CurrentIndex+1, len(bank.Words), item, input, showDef)
					continue
				}

				// Space (32) -> Replay Audio in headphones (if input is empty)
				if b == 32 {
					if len(input) == 0 {
						player.PlayAsync(target, item.Word.Definition)
						continue
					}
				}

				// Backspace (127 or 8)
				if b == 127 || b == 8 {
					if len(input) > 0 {
						input = input[:len(input)-1]
						renderPromptLine(nonce, prog.CurrentIndex+1, len(bank.Words), item, input, showDef)
					}
					continue
				}

				// Tab (9) or '?' (63) -> Hint: reveal next letter ONLY (DO NOT jump word!)
				if b == 9 || b == 63 {
					if len(input) < len(target) {
						input += string(target[len(input)])
						wordError = true
						renderPromptLine(nonce, prog.CurrentIndex+1, len(bank.Words), item, input, showDef)
					}
					continue
				}

				// Enter (13 or 10) -> SUBMIT ANSWER AND VALIDATE!
				if b == 13 || b == 10 {
					trimmedInput := strings.TrimSpace(input)
					if trimmedInput == "" {
						// If empty, replay audio as a reminder
						player.PlayAsync(target, item.Word.Definition)
						continue
					}

					if trimmedInput == target {
						// CORRECT! Advance to next word
						goto WordCompleted
					} else {
						// WRONG! Show mismatch warning log and let user retry
						wordError = true
						ts := time.Now().Format("15:04:05.000")
						fmt.Print("\033[2K\r")
						fmt.Printf("%s%s\t%sWARN%s\tauth.go:68\ttoken mismatch input=%q target_len=%d status=401 retry\r\n",
							colorGray, ts, colorRed, colorReset, trimmedInput, len(target))
						// Replay audio to reinforce memory
						player.PlayAsync(target, item.Word.Definition)
						// Reset input so user can type again
						input = ""
						renderPromptLine(nonce, prog.CurrentIndex+1, len(bank.Words), item, input, showDef)
					}
					continue
				}

				// Letter & symbol typing (a-z, A-Z, '-', ' ', '\'')
				if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '-' || b == ' ' || b == '\'' {
					charTyped := strings.ToLower(string(b))
					input += charTyped
					renderPromptLine(nonce, prog.CurrentIndex+1, len(bank.Words), item, input, showDef)
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

		ts := time.Now().Format("15:04:05.000")

		// Overwrite active line with clean English log before scrolling up
		reqType := "fresh"
		if item.IsReview {
			reqType = "retry"
		}
		fmt.Print("\033[2K\r")
		fmt.Printf("%s%s\t%sDEBUG%s\trule.go:88\tprocessed token_id=%d token=%q type=%s\r\n",
			colorGray, ts, colorYellow, colorReset, nonce, target, reqType)

		// Print verified log
		fmt.Printf("%s%s\t%sINFO%s\tauth.go:72\tsession handshake verified token=%q status=200 ms=%d\r\n",
			colorGray, ts, colorBlue, colorReset, target, ms)

		time.Sleep(80 * time.Millisecond)

		// Print realistic background ambient log occasionally
		if rand.Float32() < 0.35 {
			pattern := ambientLogs[rand.Intn(len(ambientLogs))]
			logMsg := fmt.Sprintf(pattern, rand.Intn(50000))
			hbTs := time.Now().Format("15:04:05.000")
			fmt.Printf("%s%s\t%sINFO%s\t%s\r\n", colorGray, hbTs, colorBlue, colorReset, logMsg)
		}

		queueIndex++
		prog.Save()
	}
}

func printStartupLogs() {
	now := time.Now()
	printLog(now.Add(-2*time.Second), "INFO", "bootstrap.go:42", "starting grpc service telemetry.v1 on :8080 workers=64")
	printLog(now.Add(-1600*time.Millisecond), "INFO", "postgres.go:78", "connected to postgres cluster pool=20 latency=1.1ms")
	printLog(now.Add(-1100*time.Millisecond), "INFO", "raft.go:94", "raft node initialized cluster_id=1413 role=follower")
	printLog(now.Add(-600*time.Millisecond), "INFO", "stream.go:120", "kafka consumer group active partitions=8")
	printLog(now.Add(-200*time.Millisecond), "INFO", "handler.go:88", "rule execution pipeline initialized mode=streaming")
}

func printLog(t time.Time, level, caller, msg string) {
	ts := t.Format("15:04:05.000")
	fmt.Printf("%s%s\t%s%s%s\t%s\t%s\r\n", colorGray, ts, colorBlue, level, colorReset, caller, msg)
}

func cleanDefinition(raw string) string {
	lines := strings.Split(raw, "\n")
	var candidate string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" && !strings.HasPrefix(l, "aux.") && !strings.HasPrefix(l, "abbr.") {
			candidate = l
			break
		}
	}
	if candidate == "" && len(lines) > 0 {
		candidate = strings.TrimSpace(lines[0])
	}
	candidate = strings.ReplaceAll(candidate, "\n", " ")
	runes := []rune(candidate)
	if len(runes) > 16 {
		return string(runes[:16]) + ".."
	}
	return candidate
}

func renderPromptLine(nonce, current, total int, item StudyItem, input string, showDef bool) {
	ts := time.Now().Format("15:04:05.000")
	fmt.Print("\033[2K\r") // Clear current line

	target := strings.ToLower(strings.TrimSpace(item.Word.Word))
	targetLen := len(target)
	coloredInput := formatInput(input, target)

	// Keep line compact (<= 75 columns) to strictly prevent any line-wrapping!
	if showDef {
		def := cleanDefinition(item.Word.Definition)
		fmt.Printf("%s%s\t%sDEBUG%s\trule.go:88\texpr=%q [%d/%d] len=%d token=%s█%s",
			colorGray, ts, colorYellow, colorReset, def, current, total, targetLen, coloredInput, colorReset)
	} else {
		fmt.Printf("%s%s\t%sDEBUG%s\tauth.go:64\tpayload [%d/%d] len=%d token=%s█%s",
			colorGray, ts, colorYellow, colorReset, current, total, targetLen, coloredInput, colorReset)
	}
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
			// Mismatch / typo: red
			b.WriteString(colorRed)
			b.WriteByte(ch)
			b.WriteString(colorReset)
		}
	}
	return b.String()
}
