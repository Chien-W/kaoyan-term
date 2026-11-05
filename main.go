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
	flagStart     = flag.Int("start", -1, "start from specific index (0-6356)")
	flagWrongOnly = flag.Bool("wrong", false, "practice error log tokens only")
	flagShowDef   = flag.Bool("def", true, "show business expression definition hint in logs")
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

func main() {
	flag.Parse()

	bank, err := vocab.LoadBank()
	if err != nil {
		fmt.Fprintf(os.Stderr, "init failed: %v\n", err)
		os.Exit(1)
	}

	prog := state.Load()
	if *flagStart >= 0 && *flagStart < len(bank.Words) {
		prog.CurrentIndex = *flagStart
	}

	showDef := *flagShowDef
	player := audio.NewPlayer(2) // 2: US Accent

	var words []vocab.Word
	if *flagWrongOnly {
		for _, w := range bank.Words {
			if count, ok := prog.WrongWords[w.Word]; ok && count > 0 {
				words = append(words, w)
			}
		}
		if len(words) == 0 {
			fmt.Println("error book is empty.")
			return
		}
	} else {
		words = bank.Words
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

	currentIndex := prog.CurrentIndex
	if currentIndex >= len(words) {
		currentIndex = 0
	}

	buf := make([]byte, 3)
	nonce := 9840 + currentIndex

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

		// Play English + Chinese definition in headphones
		player.PlayAsync(target, currentWord.Definition)

		input := ""
		nonce++
		renderPromptLine(nonce, currentWord, input, showDef)

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
				prog.CurrentIndex = currentIndex
				prog.Save()
				return
			}

			// Ctrl+V (22) -> Toggle definition visibility and wipe scrollback
			if b == 22 {
				showDef = !showDef
				if !showDef {
					// Purge all screen and scrollback immediately so ZERO Chinese is visible anywhere
					fmt.Print("\033[H\033[2J\033[3J")
					printStartupLogs()
				}
				renderPromptLine(nonce, currentWord, input, showDef)
				continue
			}

			// Space (32) -> Replay Bilingual Audio into headphones
			if b == 32 {
				player.PlayAsync(target, currentWord.Definition)
				continue
			}

			// Backspace (127 or 8)
			if b == 127 || b == 8 {
				if len(input) > 0 {
					input = input[:len(input)-1]
					renderPromptLine(nonce, currentWord, input, showDef)
				}
				continue
			}

			// Tab (9) or '?' (63) -> Hint: reveal next letter
			if b == 9 || b == 63 {
				if len(input) < len(target) {
					input += string(target[len(input)])
					wordError = true
					renderPromptLine(nonce, currentWord, input, showDef)
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
					renderPromptLine(nonce, currentWord, input, showDef)

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

		ms := elapsed.Milliseconds()
		if ms == 0 {
			ms = 14 + int64(rand.Intn(35))
		}

		ts := time.Now().Format("2006-01-02T15:04:05.000-0700")

		// OVERWRITE the active line with pure English log before scrolling up!
		// This guarantees that NO Chinese ever scrolls into the terminal history!
		fmt.Print("\033[2K\r")
		fmt.Printf("%s%s\t%sDEBUG%s\tevaluator/rule.go:88\tprocessed token_id=%d token=%q\r\n",
			colorGray, ts, colorYellow, colorReset, nonce, target)

		// Print verified log (pure English log)
		fmt.Printf("%s%s\t%sINFO%s\tingress/auth.go:72\tsession handshake verified nonce=%d status=200 latency_ms=%d\r\n",
			colorGray, ts, colorBlue, colorReset, nonce, ms)

		// Brief pause between words
		time.Sleep(100 * time.Millisecond)

		// Print realistic background ambient log occasionally
		if rand.Float32() < 0.35 {
			pattern := ambientLogs[rand.Intn(len(ambientLogs))]
			logMsg := fmt.Sprintf(pattern, rand.Intn(50000))
			hbTs := time.Now().Format("2006-01-02T15:04:05.000-0700")
			fmt.Printf("%s%s\t%sINFO%s\t%s\r\n", colorGray, hbTs, colorBlue, colorReset, logMsg)
		}

		currentIndex++
		prog.CurrentIndex = currentIndex
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

func renderPromptLine(nonce int, w vocab.Word, input string, showDef bool) {
	ts := time.Now().Format("2006-01-02T15:04:05.000-0700")
	fmt.Print("\033[2K\r") // Clear current line

	if showDef {
		cleanDef := strings.ReplaceAll(w.Definition, "\n", " ")
		if len(cleanDef) > 36 {
			cleanDef = cleanDef[:36] + ".."
		}
		fmt.Printf("%s%s\t%sDEBUG%s\tevaluator/rule.go:88\tevaluating rule expr=%q token=%s%s%s%s█%s",
			colorGray, ts, colorYellow, colorReset, cleanDef, colorCyan, input, colorReset, colorGreen, colorReset)
	} else {
		fmt.Printf("%s%s\t%sDEBUG%s\tingress/auth.go:64\tverifying session payload nonce=%d token=%s%s%s%s█%s",
			colorGray, ts, colorYellow, colorReset, nonce, colorCyan, input, colorReset, colorGreen, colorReset)
	}
}
