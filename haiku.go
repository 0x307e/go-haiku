package haiku

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ikawaha/kagome-dict/dict"
	"github.com/ikawaha/kagome/v2/tokenizer"
	"golang.org/x/text/unicode/norm"
	"golang.org/x/text/width"
)

var (
	reWord       = regexp.MustCompile(`^[ァ-ヾ]+$`)
	reIgnoreText = regexp.MustCompile(`[\[\]［］「」『』、。？！]`)
	reIgnoreChar = regexp.MustCompile(`[ァィゥェォャュョ]`)
	reKana       = regexp.MustCompile(`^[ァ-ヶー]+$`)
	reDigit      = regexp.MustCompile(`^[０-９]+$`)

	globalDict *dict.Dict
)

type Opt struct {
	Dict        *dict.Dict
	UserDict    *dict.UserDict
	Debug       bool
	DebugWriter io.Writer
}

func UseDict(d *dict.Dict) {
	globalDict = d
}

func dictIdx(d *dict.Dict, typ string) int {
	if ii, ok := d.ContentsMeta[typ]; ok {
		return int(ii)
	}
	return -1
}

func contains(c []string, s string) bool {
	for _, cc := range c {
		if cc == s {
			return true
		}
	}
	return false
}

func isEnd(d *dict.Dict, c []string) bool {
	idx := dictIdx(d, dict.PronunciationIndex)
	if c[0] == "接頭辞" {
		if idx >= 0 && contains(c, "御") {
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
		if idx >= 0 && c[idx] == "ノ" {
			return true
		}
		return false
	}
	idx = dictIdx(d, dict.InflectionalForm)
	if idx >= 0 && idx < len(c) {
		if c[idx] == "未然形" {
			return false
		}
		//if strings.HasPrefix(c[idx], "連用") {
		//	return false
		//}
	}
	return true
}

func isIgnore(d *dict.Dict, c []string) bool {
	return len(c) > 0 && (c[0] == "空白" || c[0] == "補助記号" || (c[0] == "記号" && c[1] == "空白"))
}

// isWord return true when the kind of the word is possible to be leading of
// sentence.
func isWord(d *dict.Dict, c []string) bool {
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

// countChars return count of characters with ignoring japanese small letters.
func countChars(s string) int {
	return len([]rune(reIgnoreChar.ReplaceAllString(s, "")))
}

// normalizeText normalizes half-width katakana to full-width and applies NFC.
// Returns the normalized text and a rune-index mapping where nfcToOrig[i]
// is the rune index in original text corresponding to rune i in normalized text.
func normalizeText(orig string) (string, []int) {
	widened := width.Widen.String(orig)
	normalized := norm.NFC.String(widened)

	wideRunes := []rune(widened)
	nfcRunes := []rune(normalized)

	nfcToOrig := make([]int, len(nfcRunes)+1)
	wi := 0
	for ni := 0; ni < len(nfcRunes); ni++ {
		nfcToOrig[ni] = wi
		if wi < len(wideRunes) && wideRunes[wi] == nfcRunes[ni] {
			wi++
		} else if wi < len(wideRunes) {
			wi++ // base character
			for wi < len(wideRunes) && unicode.Is(unicode.Mn, wideRunes[wi]) {
				wi++
			}
		}
	}
	if wi > len(wideRunes) {
		wi = len(wideRunes)
	}
	nfcToOrig[len(nfcRunes)] = wi

	return normalized, nfcToOrig
}

// splitUnknownKatakana splits an unknown katakana token into known sub-tokens.
// This handles cases where Kagome merges known and unknown katakana into a
// single unknown token (e.g. "テストミミッキュ").
// It tokenizes the surface once and returns the result if the tokenizer
// produces a more granular split with at least one known token.
func splitUnknownKatakana(t *tokenizer.Tokenizer, surface string) []tokenizer.Token {
	runes := []rune(surface)
	if len(runes) <= 1 {
		return nil
	}
	tokens := t.Analyze(surface, tokenizer.Search)
	if len(tokens) <= 1 {
		return nil
	}
	// Check that at least one token is a known word (features >= 7).
	hasKnown := false
	for _, tok := range tokens {
		if len(tok.Features()) >= 7 {
			hasKnown = true
			break
		}
	}
	if !hasKnown {
		return nil
	}
	return tokens
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
	d := opt.Dict
	if d == nil {
		d = globalDict
	}
	if d == nil {
		panic("dict is nil")
	}
	opts := []tokenizer.Option{tokenizer.OmitBosEos()}
	if opt.UserDict != nil {
		opts = append(opts, tokenizer.UserDict(opt.UserDict))
	}
	t, err := tokenizer.New(d, opts...)
	if err != nil {
		return false
	}
	text = norm.NFC.String(width.Widen.String(text))
	text = reIgnoreText.ReplaceAllString(text, " ")
	tokens := t.Tokenize(text)
	pos := 0
	r := make([]int, len(rule))
	copy(r, rule)

	var tmp []tokenizer.Token
	for _, token := range tokens {
		c := token.Features()
		if !isIgnore(d, c) {
			tmp = append(tmp, token)
		}
	}
	tokens = tmp

	// Split unknown katakana tokens into known sub-tokens
	for i := 0; i < len(tokens); i++ {
		if reKana.MatchString(tokens[i].Surface) && len(tokens[i].Features()) < 7 {
			if sub := splitUnknownKatakana(t, tokens[i].Surface); len(sub) > 0 {
				expanded := make([]tokenizer.Token, 0, len(tokens)+len(sub)-1)
				expanded = append(expanded, tokens[:i]...)
				expanded = append(expanded, sub...)
				expanded = append(expanded, tokens[i+1:]...)
				tokens = expanded
			}
		}
	}

	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		c := tok.Features()
		if reDigit.MatchString(tok.Surface) {
			return false
		}
		var y string
		if reKana.MatchString(tok.Surface) {
			y = tok.Surface
		} else if len(c) == 3 {
			y = c[2]
		} else {
			idx := dictIdx(d, dict.PronunciationIndex)
			if idx >= 0 && idx < len(c) {
				y = c[idx]
			} else {
				y = tok.Surface
			}
		}
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
		if pos >= len(rule) || (r[pos] == rule[pos] && !isWord(d, c)) {
			return false
		}
		n := countChars(y)
		r[pos] -= n
		if r[pos] == 0 {
			if !isEnd(d, c) {
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
	if len(rule) == 0 {
		return nil, nil
	}
	d := opt.Dict
	if d == nil {
		d = globalDict
	}
	if d == nil {
		panic("dict is nil")
	}
	opts := []tokenizer.Option{tokenizer.OmitBosEos()}
	if opt.UserDict != nil {
		opts = append(opts, tokenizer.UserDict(opt.UserDict))
	}
	t, err := tokenizer.New(d, opts...)
	if err != nil {
		return nil, err
	}
	origRunes := []rune(text)
	normText, nfcToOrig := normalizeText(text)
	normText = reIgnoreText.ReplaceAllString(normText, " ")
	tokens := t.Tokenize(normText)

	// Build original surfaces using rune mapping
	origSurfaces := make([]string, len(tokens))
	runePos := 0
	for i, tok := range tokens {
		tokRuneLen := utf8.RuneCountInString(tok.Surface)
		origStart := nfcToOrig[runePos]
		origEnd := nfcToOrig[runePos+tokRuneLen]
		origSurfaces[i] = string(origRunes[origStart:origEnd])
		runePos += tokRuneLen
	}

	pos := 0
	r := make([]int, len(rule))
	copy(r, rule)
	sentence := ""
	start := 0
	ambigous := 0

	// Filter ignored tokens (keep origSurfaces in sync)
	var filteredTokens []tokenizer.Token
	var filteredOrig []string
	for i, token := range tokens {
		c := token.Features()
		if !isIgnore(d, c) {
			filteredTokens = append(filteredTokens, token)
			filteredOrig = append(filteredOrig, origSurfaces[i])
		}
	}
	tokens = filteredTokens
	origSurfaces = filteredOrig

	// Merge consecutive unknown katakana, then split into known sub-tokens
	for i := 0; i < len(tokens); i++ {
		if reKana.MatchString(tokens[i].Surface) && len(tokens[i].Features()) < 7 {
			// Merge consecutive unknown katakana
			surface := tokens[i].Surface
			origSurf := origSurfaces[i]
			var j int
			for j = i + 1; j < len(tokens); j++ {
				if reKana.MatchString(tokens[j].Surface) && len(tokens[j].Features()) < 7 {
					surface += tokens[j].Surface
					origSurf += origSurfaces[j]
				} else {
					break
				}
			}
			if j > i+1 {
				tokens[i].Surface = surface
				origSurfaces[i] = origSurf
				copy(tokens[i+1:], tokens[j:])
				tokens = tokens[:len(tokens)-(j-i-1)]
				copy(origSurfaces[i+1:], origSurfaces[j:])
				origSurfaces = origSurfaces[:len(origSurfaces)-(j-i-1)]
			}

			// Split merged unknown katakana into known sub-tokens
			if sub := splitUnknownKatakana(t, tokens[i].Surface); len(sub) > 0 {
				// Build sub-origSurfaces using local rune mapping
				parentOrigSurf := origSurfaces[i]
				_, localMap := normalizeText(parentOrigSurf)
				parentOrigRunes := []rune(parentOrigSurf)
				subOrig := make([]string, len(sub))
				localRunePos := 0
				for si, st := range sub {
					stRuneLen := utf8.RuneCountInString(st.Surface)
					origStart := localMap[localRunePos]
					origEnd := localMap[localRunePos+stRuneLen]
					subOrig[si] = string(parentOrigRunes[origStart:origEnd])
					localRunePos += stRuneLen
				}

				// Splice into tokens and origSurfaces
				newTokens := make([]tokenizer.Token, 0, len(tokens)+len(sub)-1)
				newTokens = append(newTokens, tokens[:i]...)
				newTokens = append(newTokens, sub...)
				newTokens = append(newTokens, tokens[i+1:]...)
				tokens = newTokens

				newOrig := make([]string, 0, len(origSurfaces)+len(subOrig)-1)
				newOrig = append(newOrig, origSurfaces[:i]...)
				newOrig = append(newOrig, subOrig...)
				newOrig = append(newOrig, origSurfaces[i+1:]...)
				origSurfaces = newOrig
			}
		}
	}

	ret := []string{}
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		c := tok.Features()
		if (len(c) < 7 && !reKana.MatchString(tok.Surface)) || reDigit.MatchString(tok.Surface) {
			if pos > 0 || r[0] != rule[0] {
				pos = 0
				ambigous = 0
				sentence = ""
				copy(r, rule)
			}
			continue
		}
		var y string
		if reKana.MatchString(tok.Surface) {
			y = tok.Surface
		} else {
			idx := dictIdx(d, dict.PronunciationIndex)
			if idx >= 0 && idx < len(c) {
				y = c[idx]
			} else {
				y = tok.Surface
			}
		}
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
		if pos >= len(rule) || (r[pos] == rule[pos] && !isWord(d, c)) {
			pos = 0
			ambigous = 0
			sentence = ""
			copy(r, rule)
			continue
		}
		ambigous += strings.Count(y, "ッ") + strings.Count(y, "ー")
		n := countChars(y)
		r[pos] -= n
		sentence += origSurfaces[i]
		if r[pos] >= 0 && (r[pos] == 0 || r[pos]+ambigous == 0) {
			pos++
			if pos == len(r) || pos == len(r)+1 {
				if isEnd(d, c) {
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
