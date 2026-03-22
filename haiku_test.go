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

func TestFindWithOpt_KatakanaOnlyHaikuShouldBeDetected(t *testing.T) {
	opts := &Opt{
		Dict: uni.Dict(),
	}
	// カタカナ語を含む俳句が正しく検出されることを確認
	text := "クリスマスケーキを食べるプレゼント"
	result, err := FindWithOpt(text, []int{5, 7, 5}, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) == 0 {
		t.Errorf("expected haiku to be found in %q, got none", text)
	}

	if !MatchWithOpt(text, []int{5, 7, 5}, opts) {
		t.Errorf("expected %q to match 5-7-5", text)
	}
}

func TestFindWithOpt_HalfWidthKatakanaShouldBeNormalized(t *testing.T) {
	opts := &Opt{
		Dict: uni.Dict(),
	}
	// 半角カタカナが全角に正規化されて検出されることを確認
	fullWidth := "クリスマスケーキを食べるプレゼント"
	halfWidth := "ｸﾘｽﾏｽｹｰｷを食べるﾌﾟﾚｾﾞﾝﾄ"
	mixed := "ｸﾘｽﾏｽケーキを食べるプレゼント"

	for _, text := range []string{fullWidth, halfWidth, mixed} {
		result, err := FindWithOpt(text, []int{5, 7, 5}, opts)
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", text, err)
		}
		if len(result) == 0 {
			t.Errorf("expected haiku to be found in %q, got none", text)
		}
		if !MatchWithOpt(text, []int{5, 7, 5}, opts) {
			t.Errorf("expected %q to match 5-7-5", text)
		}
	}
}

func TestFindWithOpt_OriginalSurfaceShouldBePreserved(t *testing.T) {
	opts := &Opt{
		Dict: uni.Dict(),
	}
	// 検出結果が入力テキストの元の表記（半角/全角）を保持することを確認
	cases := []struct {
		text    string
		wantSub string
	}{
		{"クリスマスケーキを食べるプレゼント", "クリスマス"},
		{"ｸﾘｽﾏｽｹｰｷを食べるﾌﾟﾚｾﾞﾝﾄ", "ｸﾘｽﾏｽ"},
		{"ｸﾘｽﾏｽケーキを食べるプレゼント", "ｸﾘｽﾏｽ"},
	}
	for _, c := range cases {
		result, err := FindWithOpt(c.text, []int{5, 7, 5}, opts)
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", c.text, err)
		}
		if len(result) == 0 {
			t.Fatalf("expected haiku to be found in %q, got none", c.text)
		}
		if !strings.Contains(result[0], c.wantSub) {
			t.Errorf("expected result %q to contain %q", result[0], c.wantSub)
		}
	}
}

func TestFindWithOpt_UnknownKatakanaShouldNotBeSkipped(t *testing.T) {
	opts := &Opt{
		Dict: uni.Dict(),
	}
	// 辞書に存在しないカタカナ語（features < 7）がスキップされ、
	// 偽陽性の俳句が検出されるバグの再現テスト
	text := "検出のテスト投稿ミミッキュ検出だ"
	result, err := FindWithOpt(text, []int{5, 7, 5}, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, s := range result {
		normalized := strings.ReplaceAll(s, " ", "")
		if !strings.Contains(text, normalized) {
			t.Errorf("found sentence %q is not a substring of original text %q", s, text)
		}
		if !strings.Contains(normalized, "ミミッキュ") && strings.Contains(normalized, "テスト投稿検出") {
			t.Errorf("ミミッキュ was skipped, producing false positive: %q", s)
		}
	}
}

func TestFindWithOpt_KagomeMergedKatakanaShouldBeSplit(t *testing.T) {
	opts := &Opt{
		Dict: uni.Dict(),
	}
	// Kagome が既知+未知カタカナを1トークンに結合するケースで
	// 正しく分割されることを確認
	text := "検出のテスト投稿ミミッキュ検出だ"
	result, _ := FindWithOpt(text, []int{5, 7, 5}, opts)
	for _, s := range result {
		normalized := strings.ReplaceAll(s, " ", "")
		if !strings.Contains(text, normalized) {
			t.Errorf("result %q is not a substring of %q", normalized, text)
		}
	}
}

func TestMatchWithOpt_FullwidthBracketsIgnored(t *testing.T) {
	opts := &Opt{
		Dict: uni.Dict(),
	}
	// ASCII/全角ブラケットが正しく無視されることを確認
	for _, text := range []string{
		"[古池や]蛙飛び込む水の音",
		"［古池や］蛙飛び込む水の音",
		"「古池や」蛙飛び込む水の音",
	} {
		if !MatchWithOpt(text, []int{5, 7, 5}, opts) {
			t.Errorf("expected %q to match 5-7-5", text)
		}
	}
}

// containsSubstring は text に sub が含まれるかを確認する。
// 句読点・空白・記号を除去した上で比較する。
func containsSubstring(text, sub string) bool {
	cleanText := reIgnoreText.ReplaceAllString(text, "")
	cleanText = strings.ReplaceAll(cleanText, " ", "")
	cleanText = strings.ReplaceAll(cleanText, "　", "")
	return strings.Contains(cleanText, sub)
}
