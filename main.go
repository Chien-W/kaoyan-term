package main

import (
	"flag"
	"fmt"
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
	flagStealth   = flag.Bool("stealth", false, "启动伪装模式（输出模拟为真实后端 go test 日志）")
	flagDictation = flag.Bool("dict", false, "启动听力默写模式（隐藏英文，纯凭听力与中文拼写）")
	flagWrongOnly = flag.Bool("wrong", false, "仅背诵错题本中的高频难词")
	flagStart     = flag.Int("start", -1, "从指定单词序号开始（0-6356）")
)

const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
	colorBold   = "\033[1m"
)

func main() {
	flag.Parse()

	bank, err := vocab.LoadBank()
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载词库失败: %v\n", err)
		os.Exit(1)
	}

	prog := state.Load()
	if *flagStealth {
		prog.StealthMode = true
	}
	if *flagDictation {
		prog.DictationMode = true
	}
	if *flagStart >= 0 && *flagStart < len(bank.Words) {
		prog.CurrentIndex = *flagStart
	}

	player := audio.NewPlayer(2) // 2: 美音, 1: 英音

	var words []vocab.Word
	if *flagWrongOnly {
		for _, w := range bank.Words {
			if count, ok := prog.WrongWords[w.Word]; ok && count > 0 {
				words = append(words, w)
			}
		}
		if len(words) == 0 {
			fmt.Println("错题本目前是空的！请先正常练习。")
			return
		}
	} else {
		words = bank.Words
	}

	// Setup terminal raw mode
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "无法进入 Raw 终端模式: %v\n", err)
		return
	}

	restoreTerminal := func(clear bool) {
		_ = term.Restore(int(os.Stdin.Fd()), oldState)
		if clear {
			fmt.Print("\033[2J\033[H")
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

	currentIndex := prog.CurrentIndex
	if currentIndex >= len(words) {
		currentIndex = 0
	}

	// Clear full screen initially
	fmt.Print("\033[2J\033[H")

	buf := make([]byte, 3)

	for {
		if currentIndex >= len(words) {
			restoreTerminal(false)
			fmt.Println("\n已完成当前单词库复习！")
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
		renderWord(prog, currentWord, currentIndex, len(words), input, true)

		startTime := time.Now()
		wordError := false

		for {
			n, err := os.Stdin.Read(buf)
			if err != nil || n == 0 {
				break
			}

			b := buf[0]

			// Esc (27) or Ctrl+C (3) -> Boss Key: instant clear and exit
			if b == 27 || b == 3 {
				restoreTerminal(true)
				prog.CurrentIndex = currentIndex
				prog.Save()
				return
			}

			// Ctrl+T (20) -> Toggle Stealth Mode (clears screen completely)
			if b == 20 {
				prog.StealthMode = !prog.StealthMode
				fmt.Print("\033[2J\033[H")
				renderWord(prog, currentWord, currentIndex, len(words), input, true)
				continue
			}

			// Ctrl+D (4) -> Toggle Dictation Mode
			if b == 4 {
				prog.DictationMode = !prog.DictationMode
				renderWord(prog, currentWord, currentIndex, len(words), input, true)
				continue
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
					renderWord(prog, currentWord, currentIndex, len(words), input, false)
				}
				continue
			}

			// Tab (9) or '?' (63) -> Hint: reveal next letter
			if b == 9 || b == 63 {
				if len(input) < len(target) {
					input += string(target[len(input)])
					wordError = true
					renderWord(prog, currentWord, currentIndex, len(words), input, false)
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
					renderWord(prog, currentWord, currentIndex, len(words), input, false)

					if input == target {
						goto WordCompleted
					}
				} else {
					wordError = true
					fmt.Print("\a") // bell
				}
			}
		}

	WordCompleted:
		elapsed := time.Since(startTime).Seconds()
		if wordError {
			prog.RecordError(target)
		} else {
			prog.RecordSuccess(target)
		}

		if prog.StealthMode {
			fmt.Printf("\r\n--- PASS: TestMetricCollector/QueryPoint_%04d (%.2fs)\r\n", currentIndex+1, elapsed)
		} else {
			fmt.Printf("\r\n%s✔ PASS! (%.2fs)%s\r\n", colorGreen, elapsed, colorReset)
		}

		player.PlayAsync(target)
		time.Sleep(180 * time.Millisecond)

		currentIndex++
		prog.CurrentIndex = currentIndex
		prog.Save()
	}
}

func renderWord(prog *state.Progress, w vocab.Word, idx, total int, input string, isNewWord bool) {
	phonetic := w.USPhonetic
	if phonetic == "" {
		phonetic = w.UKPhonetic
	}
	if phonetic != "" && !strings.HasPrefix(phonetic, "/") {
		phonetic = "/" + phonetic + "/"
	}

	target := strings.ToLower(w.Word)

	if prog.StealthMode {
		// Realistic Go Test Disguise Mode (No Kaoyan/NETEM/Vocab keywords!)
		if isNewWord {
			fmt.Printf("=== RUN   TestMetricCollector/QueryPoint_%04d\r\n", idx+1)
			fmt.Printf("    collector_test.go:%d: [param] %s\r\n", 40+idx%30, w.Definition)
			fmt.Printf("    collector_test.go:%d: [token] ", 41+idx%30)
		}
		// Redraw only the input line
		fmt.Printf("\r    collector_test.go:%d: [token] %s%s%s", 41+idx%30, colorCyan, input, colorReset)
		return
	}

	// Normal Clean Code UI
	fmt.Print("\033[2J\033[H") // Clear entire screen

	dictModeStr := ""
	if prog.DictationMode {
		dictModeStr = fmt.Sprintf(" %s[听音默写]%s", colorYellow, colorReset)
	}

	fmt.Printf("%s[VOCAB-2027]%s #%d/%d  今日已背: %d%s\r\n",
		colorBold, colorReset, idx+1, total, prog.DailyCount, dictModeStr)
	fmt.Printf("%s------------------------------------------------------------%s\r\n", colorGray, colorReset)

	displayWord := ""
	for i := 0; i < len(target); i++ {
		if i < len(input) {
			displayWord += fmt.Sprintf("%s%c%s ", colorGreen, target[i], colorReset)
		} else {
			if prog.DictationMode {
				displayWord += fmt.Sprintf("%s*%s ", colorGray, colorReset)
			} else {
				displayWord += fmt.Sprintf("%s%c%s ", colorGray, target[i], colorReset)
			}
		}
	}

	fmt.Printf("单词拼写: %s\r\n", displayWord)
	fmt.Printf("国际音标: %s%s%s\r\n", colorCyan, phonetic, colorReset)
	fmt.Printf("释义内容: %s%s%s\r\n", colorBold, w.Definition, colorReset)

	if len(w.Phrases) > 0 {
		fmt.Printf("%s真题短语: %s%s\r\n", colorGray, strings.Join(w.Phrases[:2], " | "), colorReset)
	}

	fmt.Printf("%s------------------------------------------------------------%s\r\n", colorGray, colorReset)
	fmt.Printf("键入: %s%s%s", colorGreen, input, colorReset)
	fmt.Printf("\r\n\r\n%s[空格]:重播语音 | [Tab/?]:提示字母 | [Ctrl+T]:单元测试伪装 | [Esc/Ctrl+C]:老板键清屏%s\r\n", colorGray, colorReset)
}
