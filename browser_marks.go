package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// 웹 브라우저의 빠른 이동 목록(북마크)과 주소창 입력 해석.
// 목록은 사용자 설정 폴더의 TUI-playground/bookmarks.json 에 저장함 (settings.json 과 같은 폴더).

type brMark struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

const brMaxMarks = 9

var brDefaultMarks = []brMark{
	{"네이버 뉴스", "https://news.naver.com"},
	{"위키백과", "https://ko.wikipedia.org"},
	{"해커뉴스", "https://news.ycombinator.com"},
	{"Go 블로그", "https://go.dev/blog/"},
}

func brMarksPath() string {
	p := settingsPath()
	if p == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(p), "bookmarks.json")
}

// brLoadMarks 는 저장된 목록을 읽음. 파일이 없거나 깨졌으면 기본 목록, 비어 있는 목록은 그대로 비어 있음.
// 주소가 올바르지 않은 항목은 버리고, 제목과 주소에서 제어 문자를 걸러냄.
func brLoadMarks() []brMark {
	def := append([]brMark{}, brDefaultMarks...)
	p := brMarksPath()
	if p == "" {
		return def
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return def
	}
	var in []brMark
	if json.Unmarshal(data, &in) != nil {
		return def
	}
	out := []brMark{}
	for _, m := range in {
		norm, err := brNormalizeURL(m.URL)
		if err != nil || len(out) >= brMaxMarks {
			continue
		}
		t := brClip(brCollapse(brClean(m.Title)), 40)
		if t == "" {
			t = brSiteHost(norm)
		}
		out = append(out, brMark{Title: t, URL: norm})
	}
	return out
}

func brSaveMarks(ms []brMark) error {
	p := brMarksPath()
	if p == "" {
		return fmt.Errorf("설정 폴더를 알 수 없음")
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	if ms == nil {
		ms = []brMark{}
	}
	data, err := json.MarshalIndent(ms, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// brResolveInput 은 주소창에 친 글을 주소 또는 검색어로 해석함. 두 번째 값이 true 면 검색어.
// 공백이 있으면 검색어, 공백이 없으면서 '.', ':', '/' 가 있거나 localhost 면 주소, 그 밖의 한 단어는 검색어로 봄.
// (javascript:alert(1) 처럼 ':' 가 있는 것은 주소로 보내서 brNormalizeURL 이 거절하게 함)
func brResolveInput(v string) (string, bool) {
	v = strings.TrimSpace(v)
	if strings.ContainsAny(v, " \t") {
		return brSearchURL(v), true
	}
	if strings.ContainsAny(v, ".:/") || strings.EqualFold(v, "localhost") {
		return v, false
	}
	return brSearchURL(v), true
}

const brSearchBase = "https://search.naver.com/search.naver?query="

func brSearchURL(q string) string {
	q = brCollapse(brClean(q))
	if utf8.RuneCountInString(q) > 200 {
		q = string([]rune(q)[:200])
	}
	return brSearchBase + url.QueryEscape(q)
}

// ---- 목록 편집 (앱 메서드) ----

func (a *browserApp) addMark() {
	if a.cur == nil {
		a.say("저장할 페이지가 없음", ToneWarn)
		return
	}
	for _, m := range a.marks {
		if m.URL == a.cur.URL {
			a.say("이미 빠른 이동에 있음", ToneInfo)
			return
		}
	}
	if len(a.marks) >= brMaxMarks {
		a.say(fmt.Sprintf("빠른 이동은 %d개까지 저장함. 주소 화면에서 Ctrl+D 로 지울 것", brMaxMarks), ToneWarn)
		return
	}
	t := brClip(brCollapse(brClean(a.cur.Title)), 40)
	if t == "" {
		t = brSiteHost(a.cur.URL)
	}
	a.marks = append(a.marks, brMark{Title: t, URL: a.cur.URL})
	if err := brSaveMarks(a.marks); err != nil {
		a.say("빠른 이동에 추가함 (저장 실패: "+brClip(brClean(err.Error()), 60)+")", ToneWarn)
		return
	}
	a.say("빠른 이동에 추가함", ToneOk)
}

func (a *browserApp) deleteMark() {
	if a.msel < 0 || a.msel >= len(a.marks) {
		return
	}
	a.marks = append(a.marks[:a.msel], a.marks[a.msel+1:]...)
	if a.msel >= len(a.marks) {
		a.msel = len(a.marks) - 1
	}
	if err := brSaveMarks(a.marks); err != nil {
		a.say("지웠지만 저장하지 못함: "+brClip(brClean(err.Error()), 60), ToneWarn)
		return
	}
	a.say("빠른 이동에서 지움", ToneInfo)
}

// inputRows 는 주소 입력 화면의 본문: 빠른 이동 목록과 안내. 줄이 모자라면 목록을 선택 항목 주변으로 줄이고 안내를 뺌.
func (a *browserApp) inputRows(tw int) [][]seg {
	head := []seg{sgBold("빠른 이동", colorAccent)}
	var marks [][]seg
	for i, m := range a.marks {
		mk, nameSeg := "  ", sg(padR(brTrunc(m.Title, 20), 20), colorText)
		if i == a.msel {
			mk, nameSeg = "▸ ", sgBold(padR(brTrunc(m.Title, 20), 20), colorAccent)
		}
		marks = append(marks, []seg{sg(mk, colorAccent), nameSeg, sg(brTrunc(brSiteHost(m.URL), max(4, tw-24)), colorTextMuted)})
	}
	if len(marks) == 0 {
		marks = append(marks, []seg{sg("저장한 곳이 없음. 읽는 중에 b 키로 추가", colorTextMuted)})
	}
	engine := "글이 비는 페이지를 다시 그릴 브라우저(Chrome, Edge)를 찾지 못함"
	if exe := brFindEngine(); exe != "" {
		engine = brEngineName(exe) + " 로 실제 페이지처럼 그림. 방향키로 커서를 움직여 Enter 로 클릭 (v: 글만 보기)"
	}
	tail := [][]seg{
		{sg(brTrunc("단어를 입력하면 네이버에서 검색함. 주소는 점(.)이 있으면 됨", tw), colorTextMuted)},
		{sg(brTrunc(engine, tw), colorTextMuted)},
		{sg(brTrunc("↑↓ 고르기  Enter 열기  Ctrl+D 지움", tw), colorTextMuted)},
	}
	avail := max(1, a.vh-1)
	win := min(len(marks), max(3, avail-len(tail)))
	win = min(win, avail)
	tailN := min(len(tail), avail-win)
	start := 0
	if a.msel >= 0 {
		if a.msel >= start+win {
			start = a.msel - win + 1
		}
		if a.msel < start {
			start = a.msel
		}
	}
	start = max(0, min(start, len(marks)-win))
	rows := [][]seg{head}
	rows = append(rows, marks[start:start+win]...)
	rows = append(rows, tail[:tailN]...)
	return rows
}
