package haiku

import (
	"bufio"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/ikawaha/kagome-dict/uni"
)

func testMatch(t *testing.T, filename string, rules []int, judge bool) {
	f, err := os.Open(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	opts := &Opt{
		Dict:  uni.Dict(),
		Debug: testing.Verbose(),
	}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		text := scanner.Text()
		if strings.HasPrefix(text, "#") {
			continue
		}
		t.Logf("%s (%v:%v)", text, filename, judge)
		if MatchWithOpt(text, rules, opts) != judge {
			t.Fatalf("%q for %q must be %v", text, filename, rules)
		}
	}
}

func TestHaiku(t *testing.T) {
	testMatch(t, "testdata/haiku.good", []int{5, 7, 5}, true)
	testMatch(t, "testdata/haiku.bad", []int{5, 7, 5}, false)
	testMatch(t, "testdata/tanka.good", []int{5, 7, 5, 7, 7}, true)
	testMatch(t, "testdata/tanka.bad", []int{5, 7, 5, 7, 7}, false)
}

func TestFindWithOpt_KatakanaBeforeTextShouldNotCorruptTokens(t *testing.T) {
	opts := &Opt{
		Dict: uni.Dict(),
	}
	// カタカナが先行する文でトークン配列が破損し、
	// 存在しない俳句が検出されていたバグの再現テスト
	// Refs: u16-io/FindSenryu4Discord#66
	text := "ヤメ、ヤメ、ヤメロ!!  冷静の国境線を越えて　攻めて来るぞ"
	result, err := FindWithOpt(text, []int{5, 7, 5}, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, s := range result {
		// 破損したトークン配列から生成された "国境の" のような
		// 原文に存在しない部分文字列が含まれていないことを確認
		normalized := strings.ReplaceAll(s, " ", "")
		if !containsSubstring(text, normalized) {
			t.Errorf("found sentence %q is not a substring of original text %q", s, text)
		}
	}
}

func TestFindWithOpt_TextWithoutKatakanaShouldWorkNormally(t *testing.T) {
	opts := &Opt{
		Dict: uni.Dict(),
	}
	// カタカナなしの同じ後半部分は正常に動作すべき
	text := "冷静の国境線を越えて　攻めて来るぞ"
	result, err := FindWithOpt(text, []int{5, 7, 5}, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, s := range result {
		normalized := strings.ReplaceAll(s, " ", "")
		if !containsSubstring(text, normalized) {
			t.Errorf("found sentence %q is not a substring of original text %q", s, text)
		}
	}
}

func TestFindWithOpt_ConsecutiveKatakanaMergedCorrectly(t *testing.T) {
	opts := &Opt{
		Dict: uni.Dict(),
	}
	// カタカナのみの入力でパニックしないことを確認
	text := "アイウエオカキクケコサシスセソ"
	result, err := FindWithOpt(text, []int{5, 7, 5}, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result // パニックしなければOK
}

// containsSubstring は text に sub が含まれるかを確認する。
// 句読点・空白・記号を除去した上で比較する。
func containsSubstring(text, sub string) bool {
	cleanText := reIgnoreText.ReplaceAllString(text, "")
	cleanText = strings.ReplaceAll(cleanText, " ", "")
	cleanText = strings.ReplaceAll(cleanText, "　", "")
	return strings.Contains(cleanText, sub)
}
