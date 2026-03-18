package haiku

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/yukihir0/mecab-go"
)

var (
	reWord       = regexp.MustCompile(`^[ァ-ヾ]+$`)
	reIgnoreText = regexp.MustCompile(`[\[\]「」『』、。？！]`)
	reIgnoreChar = regexp.MustCompile(`[ァィゥェォャュョ]`)
	reKana       = regexp.MustCompile(`^[ァ-タダ-ヶ]+$`)
)

// MeCab feature format: 品詞,品詞細分類1,品詞細分類2,品詞細分類3,活用型,活用形,原形,読み,発音
// Index:                 0    1          2          3          4     5     6    7   8
const (
	mecabInflectionalFormIdx = 5
	mecabPronunciationIdx    = 8
)

type Opt struct {
	DicDir      string
	Debug       bool
	DebugWriter io.Writer
}

func contains(c []string, s string) bool {
	for _, cc := range c {
		if cc == s {
			return true
		}
	}
	return false
}

func isEnd(c []string) bool {
	if c[0] == "接頭辞" || c[0] == "接頭詞" {
		// 敬称接頭辞（ご/お/御）はセグメント末尾にならない
		if contains(c, "御") || contains(c, "ご") || contains(c, "お") {
			return false
		}
		return true
	}
	if c[1] == "非自立" {
		if c[0] == "名詞" {
			return true
		}
		if c[0] == "動詞" {
			return true
		}
		return false
	}
	if mecabInflectionalFormIdx < len(c) && c[mecabInflectionalFormIdx] != "*" {
		if c[mecabInflectionalFormIdx] == "未然形" {
			return false
		}
	}
	return true
}

func isIgnore(c []string) bool {
	return len(c) > 0 && (c[0] == "空白" || c[0] == "補助記号" || (c[0] == "記号" && c[1] == "空白"))
}

// isWord return true when the kind of the word is possible to be leading of
// sentence.
func isWord(c []string) bool {
	if c[0] != "名詞" && c[1] == "非自立" {
		return false
	}
	for _, f := range []string{"名詞", "形容詞", "形容動詞", "副詞", "連体詞", "接続詞", "感動詞", "接頭詞", "フィラー"} {
		if f == c[0] && c[1] != "接尾" {
			return true
		}
	}
	if c[0] == "接頭辞" || (c[0] == "接続詞" && c[1] == "名詞接続") {
		return false
	}
	if c[0] == "形状詞" && c[1] != "助動詞語幹" {
		return true
	}
	if c[0] == "代名詞" {
		return true
	}
	if c[0] == "記号" && c[1] == "一般" {
		return true
	}
	if c[0] == "助詞" && c[1] != "副助詞" && c[1] != "準体助詞" && c[1] != "終助詞" && c[1] != "係助詞" && c[1] != "格助詞" && c[1] != "接続助詞" && c[1] != "連体化" && c[1] != "副助詞／並立助詞／終助詞" {
		return true
	}
	if c[0] == "動詞" && c[1] != "接尾" && c[1] != "非自立" {
		return true
	}
	if c[0] == "カスタム人名" || c[0] == "カスタム名詞" {
		return true
	}
	return false
}

// pronunciation extracts the pronunciation from MeCab features.
func pronunciation(surface string, c []string) string {
	if reKana.MatchString(surface) {
		return surface
	}
	if mecabPronunciationIdx < len(c) && c[mecabPronunciationIdx] != "*" {
		return c[mecabPronunciationIdx]
	}
	// fallback to reading
	if mecabPronunciationIdx-1 < len(c) && c[mecabPronunciationIdx-1] != "*" {
		return c[mecabPronunciationIdx-1]
	}
	return surface
}

// countChars return count of characters with ignoring japanese small letters.
func countChars(s string) int {
	return len([]rune(reIgnoreChar.ReplaceAllString(s, "")))
}

// Match return true when text matches with rule(s).
func Match(text string, rule []int) bool {
	return MatchWithOpt(text, rule, &Opt{})
}

// MatchWithOpt return true when text matches with rule(s).
func MatchWithOpt(text string, rule []int, opt *Opt) bool {
	if len(rule) == 0 {
		return false
	}
	text = reIgnoreText.ReplaceAllString(text, " ")
	tokens, err := mecab.Parse(text)
	if err != nil {
		if opt.Debug {
			if opt.DebugWriter != nil {
				fmt.Fprintln(opt.DebugWriter, "mecab.Parse error:", err)
			} else {
				fmt.Fprintln(os.Stderr, "mecab.Parse error:", err)
			}
		}
		return false
	}

	// filter ignored tokens
	var filtered []mecab.Node
	for _, tok := range tokens {
		c := strings.Split(tok.Feature, ",")
		if len(c) > 0 && !isIgnore(c) {
			filtered = append(filtered, tok)
		}
	}
	tokens = filtered

	pos := 0
	r := make([]int, len(rule))
	copy(r, rule)

	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		c := strings.Split(tok.Feature, ",")
		if len(c) == 0 {
			continue
		}
		y := pronunciation(tok.Surface, c)
		if opt.Debug {
			if opt.DebugWriter != nil {
				fmt.Fprintln(opt.DebugWriter, c, y)
			} else {
				fmt.Fprintln(os.Stderr, c, y)
			}
		}
		if !reWord.MatchString(y) {
			if y == "、" {
				continue
			}
			return false
		}
		if pos >= len(rule) || (r[pos] == rule[pos] && !isWord(c)) {
			return false
		}
		n := countChars(y)
		r[pos] -= n
		if r[pos] == 0 {
			if !isEnd(c) {
				return false
			}
			pos++
			if pos == len(r) && i == len(tokens)-1 {
				return true
			}
		}
	}
	return false
}

func FindWithOpt(text string, rule []int, opt *Opt) ([]string, error) {
	if opt == nil {
		opt = &Opt{}
	}
	if len(rule) == 0 {
		return nil, nil
	}
	text = reIgnoreText.ReplaceAllString(text, " ")
	tokens, err := mecab.Parse(text)
	if err != nil {
		return []string{}, err
	}

	// filter ignored tokens
	var filtered []mecab.Node
	for _, tok := range tokens {
		c := strings.Split(tok.Feature, ",")
		if len(c) > 0 && !isIgnore(c) {
			filtered = append(filtered, tok)
		}
	}
	tokens = filtered

	pos := 0
	r := make([]int, len(rule))
	copy(r, rule)
	sentence := ""
	start := 0
	ambigous := 0

	for i := 0; i < len(tokens); i++ {
		if reKana.MatchString(tokens[i].Surface) {
			surface := tokens[i].Surface
			var j int
			for j = i + 1; j < len(tokens); j++ {
				if reKana.MatchString(tokens[j].Surface) {
					surface += tokens[j].Surface
				} else {
					break
				}
			}
			tokens[i].Surface = surface
			for k := 0; k < (j - i); k++ {
				if i+1+k < len(tokens) && j+k < len(tokens) {
					tokens[i+1+k] = tokens[j+k]
				}
			}
			tokens = tokens[:len(tokens)-(j-i)+1]
			i = j
		}
	}

	ret := []string{}
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		c := strings.Split(tok.Feature, ",")
		if len(c) == 0 || isIgnore(c) {
			continue
		}
		y := pronunciation(tok.Surface, c)
		if !reWord.MatchString(y) {
			if y == "、" {
				continue
			}
			pos = 0
			ambigous = 0
			sentence = ""
			copy(r, rule)
			continue
		}
		if pos >= len(rule) || (r[pos] == rule[pos] && !isWord(c)) {
			pos = 0
			ambigous = 0
			sentence = ""
			copy(r, rule)
			continue
		}
		ambigous += strings.Count(y, "ッ") + strings.Count(y, "ー")
		n := countChars(y)
		r[pos] -= n
		sentence += tok.Surface
		if r[pos] >= 0 && (r[pos] == 0 || r[pos]+ambigous == 0) {
			pos++
			if pos == len(r) || pos == len(r)+1 {
				if isEnd(c) {
					ret = append(ret, sentence)
					start = i + 1
				}
				pos = 0
				ambigous = 0
				sentence = ""
				copy(r, rule)
				continue
			}
			sentence += " "
		} else if r[pos] < 0 {
			i = start + 1
			start++
			pos = 0
			ambigous = 0
			sentence = ""
			copy(r, rule)
		}
	}
	return ret, nil
}

// Find returns sentences that text matches with rule(s).
func Find(text string, rule []int) []string {
	res, _ := FindWithOpt(text, rule, &Opt{})
	return res
}
