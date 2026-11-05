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
	flagStart     = flag.Int("start", -1, "从指定单词序号开始（0-6356）")
	flagWrongOnly = flag.Bool("wrong", false, "仅复习错题本中的词汇")
)

const (
	colorReset  = "\033[0m"
	colorCyan   = "\033[36m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorGray   = "\033[90m"
)

var dummyHeartbeats = []string{
	"pipeline/worker.go:104\tbatch buffer flushed stream=telemetry_raw events=128 lag=0",
	"cluster/heartbeat.go:52\tcluster node health ping ack status=200 latency_ms=1.4",
	"storage/pool.go:88\tconnection pool health check active=12 idle=8 max=32",
	"gateway/router.go:145\tingress rate limit bucket refreshed tokens=5000",
}

func main() {
	flag.Parse()

	bank, err := vocab.LoadBank()
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化失败: %v\n", err)
		os.Exit(1)
	}

	prog := state.Load()
	if *flagStart >= 0 && *flagStart < len(bank.Words) {
		prog.CurrentIndex = *flagStart
	}

	player := audio.NewPlayer(2) // 2: 美音

	var words []vocab.Word
	if *flagWrongOnly {
		for _, w := range bank.Words {
			if count, ok := prog.WrongWords[w.Word]; ok && count > 0 {
				words = append(words, w)
			}
		}
		if len(words) == 0 {
			fmt.Println("当前错题本为空。")
			return
		}
	} else {
		words = bank.Words
	}

	// Setup raw terminal mode
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "终端模式初始化失败: %v\n", err)
		return
	}

	restoreTerminal := func(clear bool) {
		_ = term.Restore(int(os.Stdin.Fd()), oldState)
		if clear {
			fmt.Print("\033[H\033[2J\033[3J") // Completely wipe screen and scrollback
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

	// Clear screen completely once at startup
	fmt.Print("\033[H\033[2J\033[3J")

	// Print realistic microservice startup logs
	now := time.Now()
	printStartupLog(now.Add(-2 * time.Second), "INFO", "server/bootstrap.go:42", "initializing metric collector worker_pool=32 max_inflight=1000")
	printStartupLog(now.Add(-1500 * time.Millisecond), "INFO", "storage/postgres.go:78", "connected to primary database pool_size=20 latency_ms=1.1")
	printStartupLog(now.Add(-800 * time.Millisecond), "INFO", "pipeline/stream.go:94", "kafka consumer group rebalanced partition_count=8")
	printStartupLog(now.Add(-200 * time.Millisecond), "INFO", "dispatch/handler.go:110", "rule evaluation engine started mode=streaming")

	currentIndex := prog.CurrentIndex
	if currentIndex >= len(words) {
		currentIndex = 0
	}

	buf := make([]byte, 3)

	for {
		if currentIndex >= len(words) {
			restoreTerminal(false)
			fmt.Println("\r\nservice stopped: all tasks completed.")
			return
		}

		currentWord := words[currentIndex]
		target := strings.ToLower(strings.TrimSpace(currentWord.Word))
		if target == "" {
			currentIndex++
			continue
		}

		player.PlayAsync(target)

		input := ""
		renderLogLine(currentWord, input)

		startTime := time.Now()
		wordError := false

		for {
			n, err := os.Stdin.Read(buf)
			if err != nil || n == 0 {
				break
			}

			b := buf[0]

			// Esc (27) or Ctrl+C (3) -> Boss Key: instant wipe
			if b == 27 || b == 3 {
				restoreTerminal(true)
				prog.CurrentIndex = currentIndex
				prog.Save()
				return
			}

			// Space (32) -> Replay Audio
			if b == 32 {
				player.PlayAsync(target)
				continue
			}

			// Backspace (127 or 8)
			if b == 127 || b == 8 {
				if len(input) > 0 {
					input = input[:len(input)-1]
					renderLogLine(currentWord, input)
				}
				continue
			}

			// Tab (9) or '?' (63) -> Hint: reveal next letter
			if b == 9 || b == 63 {
				if len(input) < len(target) {
					input += string(target[len(input)])
					wordError = true
					renderLogLine(currentWord, input)
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
					renderLogLine(currentWord, input)

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
			prog.RecordSuccess(target)
		}

		// Print realistic match success log
		ms := elapsed.Milliseconds()
		if ms == 0 {
			ms = 18 + int64(rand.Intn(40))
		}
		ts := time.Now().Format("2006-01-02T15:04:05.000-0700")
		fmt.Printf("\r\n%s%s\t%sINFO%s\tevaluator/matcher.go:94\trule token verified status=200 target=%q latency_ms=%d\r\n",
			colorGray, ts, colorBlue, colorReset, target, ms)

		player.PlayAsync(target)
		time.Sleep(120 * time.Millisecond)

		// Occasionally print a background heartbeat log line
		if rand.Float32() < 0.25 {
			hb := dummyHeartbeats[rand.Intn(len(dummyHeartbeats))]
			hbTs := time.Now().Format("2006-01-02T15:04:05.000-0700")
			fmt.Printf("%s%s\t%sINFO%s\t%s\r\n", colorGray, hbTs, colorBlue, colorReset, hb)
		}

		currentIndex++
		prog.CurrentIndex = currentIndex
		prog.Save()
	}
}

func printStartupLog(t time.Time, level, caller, msg string) {
	ts := t.Format("2006-01-02T15:04:05.000-0700")
	fmt.Printf("%s%s\t%s%s%s\t%s\t%s\r\n", colorGray, ts, colorBlue, level, colorReset, caller, msg)
}

func renderLogLine(w vocab.Word, input string) {
	ts := time.Now().Format("2006-01-02T15:04:05.000-0700")
	cleanDef := strings.ReplaceAll(w.Definition, "\n", " ")
	if len(cleanDef) > 40 {
		cleanDef = cleanDef[:40] + "..."
	}

	fmt.Print("\033[2K\r") // Clear current line
	fmt.Printf("%s%s\t%sDEBUG%s\tevaluator/matcher.go:88\tevaluating expr=%q token=%s%s%s%s█%s",
		colorGray, ts, colorYellow, colorReset, cleanDef, colorCyan, input, colorReset, colorGreen, colorReset)
}
