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
	flagStealth   = flag.Bool("stealth", false, "启动伪装模式（输出模拟为 go test 单元测试输出）")
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
	colorRed    = "\033[31m"
	colorBold   = "\033[1m"
)

func main() {
	flag.Parse()

	bank, err := vocab.LoadBank()
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载考研词库失败: %v\n", err)
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

	// Filter words if wrong only
	var words []vocab.Word
	if *flagWrongOnly {
		for _, w := range bank.Words {
			if count, ok := prog.WrongWords[w.Word]; ok && count > 0 {
				words = append(words, w)
			}
		}
		if len(words) == 0 {
			fmt.Println("错题本目前是空的！太棒了，请先正常背诵新词。")
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

	// Restore terminal on exit
	restoreTerminal := func(clear bool) {
		_ = term.Restore(int(os.Stdin.Fd()), oldState)
		if clear {
			fmt.Print("\033[2J\033[H") // Clear screen
		}
	}

	// Catch SIGINT / SIGTERM for safe cleanup
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

	// Game loop
	currentIndex := prog.CurrentIndex
	if currentIndex >= len(words) {
		currentIndex = 0
	}

	buf := make([]byte, 3)

	for {
		if currentIndex >= len(words) {
			restoreTerminal(false)
			fmt.Println("\n恭喜！你已经完成了全套考研词库的复习！")
			return
		}

		currentWord := words[currentIndex]
		target := strings.ToLower(strings.TrimSpace(currentWord.Word))
		if target == "" {
			currentIndex++
			continue
		}

		// Play audio when word appears
		player.PlayAsync(target)

		input := ""
		renderScreen(prog, currentWord, currentIndex, len(words), input, player.IsMuted())

		startTime := time.Now()
		wordError := false

		// Input loop for this word
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil || n == 0 {
				break
			}

			b := buf[0]

			// Ctrl+C (3) or Esc (27) -> Boss Key: instant clear and exit
			if b == 3 || b == 27 {
				restoreTerminal(true)
				prog.CurrentIndex = currentIndex
				prog.Save()
				return
			}

			// Ctrl+S (19) -> Toggle Stealth Mode
			if b == 19 {
				prog.StealthMode = !prog.StealthMode
				renderScreen(prog, currentWord, currentIndex, len(words), input, player.IsMuted())
				continue
			}

			// Ctrl+D (4) -> Toggle Dictation Mode
			if b == 4 {
				prog.DictationMode = !prog.DictationMode
				renderScreen(prog, currentWord, currentIndex, len(words), input, player.IsMuted())
				continue
			}

			// Ctrl+M (13 is enter, 14 is ctrl+n; ctrl+m is carriage return in raw mode, so use Tab / Ctrl+P for sound)
			// Space (32) -> Replay Audio
			if b == 32 {
				player.PlayAsync(target)
				continue
			}

			// Backspace (127 or 8)
			if b == 127 || b == 8 {
				if len(input) > 0 {
					input = input[:len(input)-1]
					renderScreen(prog, currentWord, currentIndex, len(words), input, player.IsMuted())
				}
				continue
			}

			// Tab (9) or '?' (63) -> Hint: reveal next letter
			if b == 9 || b == 63 {
				if len(input) < len(target) {
					input += string(target[len(input)])
					wordError = true
					renderScreen(prog, currentWord, currentIndex, len(words), input, player.IsMuted())
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
					renderScreen(prog, currentWord, currentIndex, len(words), input, player.IsMuted())

					if input == target {
						goto WordCompleted
					}
				} else {
					// Typed wrong letter
					wordError = true
					// Flash error or ignore
					fmt.Print("\a") // terminal bell
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
			fmt.Printf("\r\n--- PASS: TestKaoyanVocab/NETEM_2027/Word_%04d (%.2fs)\r\n", currentIndex+1, elapsed)
		} else {
			fmt.Printf("\r\n%s✔ PASS! (%.2fs)%s\r\n", colorGreen, elapsed, colorReset)
		}

		// Replay audio once more upon completion
		player.PlayAsync(target)
		time.Sleep(200 * time.Millisecond)

		currentIndex++
		prog.CurrentIndex = currentIndex
		prog.Save()
	}
}

func renderScreen(prog *state.Progress, w vocab.Word, idx, total int, input string, isMuted bool) {
	phonetic := w.USPhonetic
	if phonetic == "" {
		phonetic = w.UKPhonetic
	}
	if phonetic != "" && !strings.HasPrefix(phonetic, "/") {
		phonetic = "/" + phonetic + "/"
	}

	target := strings.ToLower(w.Word)

	if prog.StealthMode {
		// Go Test Disguise Mode
		fmt.Print("\033[2K\r") // Clear current line
		fmt.Printf("=== RUN   TestKaoyanVocab/NETEM_2027/Word_%04d\r\n", idx+1)
		fmt.Printf("    vocab_test.go:%d: [def] %s  [ipa] %s\r\n", 40+idx%50, w.Definition, phonetic)
		if len(w.Phrases) > 0 {
			fmt.Printf("    vocab_test.go:%d: [collocations] %s\r\n", 41+idx%50, strings.Join(w.Phrases[:1], "; "))
		}
		// Input line
		fmt.Printf("    vocab_test.go:%d: [input] %s%s%s", 42+idx%50, colorCyan, input, colorReset)
		return
	}

	// Normal Clean Terminal Mode
	fmt.Print("\033[2J\033[H") // Clear screen and move cursor to home

	// Status header
	dictModeStr := ""
	if prog.DictationMode {
		dictModeStr = fmt.Sprintf(" %s[听音默写模式]%s", colorYellow, colorReset)
	}

	fmt.Printf("%s[NETEM-2027 考研词汇]%s #%d/%d  今日已背: %d%s\r\n",
		colorBold, colorReset, idx+1, total, prog.DailyCount, dictModeStr)
	fmt.Printf("%s------------------------------------------------------------%s\r\n", colorGray, colorReset)

	// Word display (in dictation mode, hide target letters until typed)
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
	fmt.Printf("大纲释义: %s%s%s\r\n", colorBold, w.Definition, colorReset)

	if len(w.Phrases) > 0 {
		fmt.Printf("%s常用搭配: %s%s\r\n", colorGray, strings.Join(w.Phrases[:2], " | "), colorReset)
	}

	fmt.Printf("%s------------------------------------------------------------%s\r\n", colorGray, colorReset)
	fmt.Printf("键入: %s%s%s", colorGreen, input, colorReset)

	// Bottom shortcuts hint
	fmt.Printf("\r\n\r\n%s[空格]:重听发音 | [Tab/?]:提示字母 | [Ctrl+S]:切换单元测试伪装 | [Esc/Ctrl+C]:老板键清屏%s\r\n", colorGray, colorReset)
}
