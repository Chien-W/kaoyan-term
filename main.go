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
)

var ambientLogs = []string{
	"cluster/raft.go:184\tcommit log entry index=%d term=14 leader_id=\"node-10.200.25.165\"",
	"storage/wal.go:92\tsync wal segment flushed_bytes=16384 duration_us=182",
	"telemetry/metric.go:142\thistogram bucket recorded metric=\"net_ingress_bytes\" p99_latency_ms=1.4",
	"network/pool.go:68\treused idle tcp keepalive socket fd=%d remote=\"10.200.25.165:31805\"",
	"pipeline/worker.go:119\tconsumer partition rebalance state=stable assigned=[0,1,2,3]",
	"cache/redis.go:54\tkey buffer evicted namespace=\"alarm_cache\" freed_keys=128",
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

	// Index bank words for fast lookup
	wordMap := make(map[string]vocab.Word, len(bank.Words))
	for _, w := range bank.Words {
		wordMap[strings.ToLower(w.Word)] = w
	}

	// Build queue: Due reviews first, then new words
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
				queue = append(queue, StudyItem{Word: w, IsReview: true})
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
				queue = append(queue, StudyItem{Word: w, IsReview: true})
			}
		}
		// Then new words starting from CurrentIndex
		for i := prog.CurrentIndex; i < len(bank.Words); i++ {
			queue = append(queue, StudyItem{Word: bank.Words[i], IsReview: false})
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

	buf := make([]byte, 3)
	nonce := 9840 + prog.CurrentIndex
	queueIndex := 0

	for {
		if queueIndex >= len(queue) {
			restoreTerminal(false)
			fmt.Println("\r\nservice stopped: all scheduled tasks completed.")
			return
		}

		item := queue[queueIndex]
		target := strings.ToLower(strings.TrimSpace(item.Word.Word))
		if target == "" {
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

			b := buf[0]

			// Esc (27) or Ctrl+C (3) -> Boss Key: instant full wipe
			if b == 27 || b == 3 {
				restoreTerminal(true)
				prog.Save()
				return
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
				ts := time.Now().Format("2006-01-02T15:04:05.000-0700")
				remaining := len(bank.Words) - len(prog.Records)
				if remaining < 0 {
					remaining = 0
				}
				fmt.Print("\033[2K\r")
				fmt.Printf("%s%s\t%sINFO%s\ttelemetry/metric.go:50\tsync stats offset=%d/%d left=%d today_new=%d/%d today_rev=%d due_queue=%d\r\n",
					colorGray, ts, colorBlue, colorReset,
					prog.CurrentIndex+1, len(bank.Words), remaining,
					prog.DailyNewCount, prog.DailyGoal, prog.DailyRevCount, len(prog.GetDueReviewWords()))
				renderPromptLine(nonce, prog.CurrentIndex+1, len(bank.Words), item, input, showDef)
				continue
			}

			// Space (32) -> Replay Bilingual Audio into headphones
			if b == 32 {
				player.PlayAsync(target, item.Word.Definition)
				continue
			}

			// Backspace (127 or 8)
			if b == 127 || b == 8 {
				if len(input) > 0 {
					input = input[:len(input)-1]
					renderPromptLine(nonce, prog.CurrentIndex+1, len(bank.Words), item, input, showDef)
				}
				continue
			}

			// Tab (9) or '?' (63) -> Hint: reveal next letter
			if b == 9 || b == 63 {
				if len(input) < len(target) {
					input += string(target[len(input)])
					wordError = true
					renderPromptLine(nonce, prog.CurrentIndex+1, len(bank.Words), item, input, showDef)
					if input == target {
						goto WordCompleted
					}
				}
				continue
			}

			// Letter typing
			if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '-' || b == ' ' || b == '\'' {
				charTyped := strings.ToLower(string(b))
				expectedChar := ""
				if len(input) < len(target) {
					expectedChar = string(target[len(input)])
				}

				if expectedChar != "" && charTyped == expectedChar {
					input += charTyped
					renderPromptLine(nonce, prog.CurrentIndex+1, len(bank.Words), item, input, showDef)

					if input == target {
						goto WordCompleted
					}
				} else {
					wordError = true
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

		ts := time.Now().Format("2006-01-02T15:04:05.000-0700")

		// Overwrite active line with pure English log before scrolling up
		reqType := "fresh"
		if item.IsReview {
			reqType = "retry"
		}
		fmt.Print("\033[2K\r")
		fmt.Printf("%s%s\t%sDEBUG%s\tevaluator/rule.go:88\tprocessed token_id=%d token=%q type=%s\r\n",
			colorGray, ts, colorYellow, colorReset, nonce, target, reqType)

		// Print verified log
		fmt.Printf("%s%s\t%sINFO%s\tingress/auth.go:72\tsession handshake verified nonce=%d status=200 latency_ms=%d\r\n",
			colorGray, ts, colorBlue, colorReset, nonce, ms)

		time.Sleep(100 * time.Millisecond)

		// Print realistic background ambient log occasionally
		if rand.Float32() < 0.35 {
			pattern := ambientLogs[rand.Intn(len(ambientLogs))]
			logMsg := fmt.Sprintf(pattern, rand.Intn(50000))
			hbTs := time.Now().Format("2006-01-02T15:04:05.000-0700")
			fmt.Printf("%s%s\t%sINFO%s\t%s\r\n", colorGray, hbTs, colorBlue, colorReset, logMsg)
		}

		queueIndex++
		prog.Save()
	}
}

func printStartupLogs() {
	now := time.Now()
	printLog(now.Add(-2*time.Second), "INFO", "server/bootstrap.go:42", "starting grpc service telemetry.v1 on :8080 max_workers=64")
	printLog(now.Add(-1600*time.Millisecond), "INFO", "storage/postgres.go:78", "connected to postgres cluster pool_size=20 latency_ms=1.12")
	printLog(now.Add(-1100*time.Millisecond), "INFO", "cluster/raft.go:94", "raft node initialized cluster_id=1413 role=follower")
	printLog(now.Add(-600*time.Millisecond), "INFO", "pipeline/stream.go:120", "kafka partition stream consumer group active partitions=8")
	printLog(now.Add(-200*time.Millisecond), "INFO", "dispatch/handler.go:88", "rule execution pipeline initialized mode=streaming")
}

func printLog(t time.Time, level, caller, msg string) {
	ts := t.Format("2006-01-02T15:04:05.000-0700")
	fmt.Printf("%s%s\t%s%s%s\t%s\t%s\r\n", colorGray, ts, colorBlue, level, colorReset, caller, msg)
}

func renderPromptLine(nonce, current, total int, item StudyItem, input string, showDef bool) {
	ts := time.Now().Format("2006-01-02T15:04:05.000-0700")
	fmt.Print("\033[2K\r") // Clear current line

	reqType := "fresh"
	if item.IsReview {
		reqType = "retry"
	}

	if showDef {
		cleanDef := strings.ReplaceAll(item.Word.Definition, "\n", " ")
		if len(cleanDef) > 32 {
			cleanDef = cleanDef[:32] + ".."
		}
		// Notice offset="%d/%d" naturally conveys current / total progress!
		fmt.Printf("%s%s\t%sDEBUG%s\tevaluator/rule.go:88\tevaluating rule expr=%q offset=%q type=%s token=%s%s%s%s█%s",
			colorGray, ts, colorYellow, colorReset, cleanDef, fmt.Sprintf("%d/%d", current, total), reqType, colorCyan, input, colorReset, colorGreen, colorReset)
	} else {
		fmt.Printf("%s%s\t%sDEBUG%s\tingress/auth.go:64\tverifying session payload offset=%q type=%s token=%s%s%s%s█%s",
			colorGray, ts, colorYellow, colorReset, fmt.Sprintf("%d/%d", current, total), reqType, colorCyan, input, colorReset, colorGreen, colorReset)
	}
}
