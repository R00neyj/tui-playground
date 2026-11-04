package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/text/encoding/korean"
)

// 웹 브라우저 테스트. 진짜 인터넷에는 나가지 않음 (httptest 서버나 가짜 browserFetch 만 씀).

// ---- 도우미 ----

func brParse(t *testing.T, doc, raw string) *brPage {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return brBuildPage([]byte(doc), "text/html; charset=utf-8", false, u)
}

// brText 는 문서를 폭 w 로 펼쳐 줄 글자들을 \n 으로 이어 돌려줌
func brText(p *brPage, w int) string {
	var out []string
	for _, l := range brLayoutPage(p, w) {
		out = append(out, brLineText(l))
	}
	return strings.Join(out, "\n")
}

func brMustContain(t *testing.T, name, s string, subs ...string) {
	t.Helper()
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			t.Errorf("%s: %q 가 없음\n%s", name, sub, s)
		}
	}
}

func brMustNotContain(t *testing.T, name, s string, subs ...string) {
	t.Helper()
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			t.Errorf("%s: %q 가 들어 있으면 안 됨\n%s", name, sub, s)
		}
	}
}

func brWithFetch(t *testing.T, f func(ctx context.Context, raw string) (*brPage, error)) {
	t.Helper()
	old := browserFetch
	browserFetch = f
	t.Cleanup(func() { browserFetch = old })
}

func brWithTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	old := browserTimeout
	browserTimeout = d
	t.Cleanup(func() { browserTimeout = old })
}

func brKey(s string) tea.KeyMsg {
	switch s {
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "pgdown":
		return tea.KeyMsg{Type: tea.KeyPgDown}
	case "pgup":
		return tea.KeyMsg{Type: tea.KeyPgUp}
	case " ":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	}
	return key(s)
}

// brDrive 는 명령(Batch 포함)을 실행하고 brDoneMsg 가 오면 앱에 전달함. 틱 같은 나머지 메시지는 버림.
func brDrive(t *testing.T, a *browserApp, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		return
	}
	var cmds []tea.Cmd
	if b, ok := cmd().(tea.BatchMsg); ok {
		cmds = b
	} else {
		return
	}
	ch := make(chan tea.Msg, len(cmds))
	for _, c := range cmds {
		if c == nil {
			continue
		}
		go func(c tea.Cmd) { ch <- c() }(c)
	}
	deadline := time.After(3 * time.Second)
	for range cmds {
		select {
		case m := <-ch:
			if d, ok := m.(brDoneMsg); ok {
				a.Update(d)
				return
			}
		case <-deadline:
			t.Fatal("brDoneMsg 가 오지 않음")
		}
	}
}

// brOpen 은 주소 입력 → Enter 까지 하고 결과를 반영함
func brOpen(t *testing.T, a *browserApp, addr string) {
	t.Helper()
	if a.mode != brInput {
		a.Update(key("o"))
	}
	a.input.SetValue(addr)
	_, cmd := a.Update(key("enter"))
	brDrive(t, a, cmd)
}

type brFakeWeb struct {
	mu    sync.Mutex
	pages map[string]string
	got   []string
}

func (w *brFakeWeb) fetch(ctx context.Context, raw string) (*brPage, error) {
	w.mu.Lock()
	w.got = append(w.got, raw)
	doc, ok := w.pages[raw]
	w.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("HTTP 404 %s", http.StatusText(404))
	}
	u, _ := url.Parse(raw)
	return brBuildPage([]byte(doc), "text/html; charset=utf-8", false, u), nil
}

func (w *brFakeWeb) fetched() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string{}, w.got...)
}

func brLongDoc(n int) string {
	var b strings.Builder
	b.WriteString("<html><head><title>긴 글</title></head><body><article>")
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "<p>문단 %d 번입니다. 이것은 스크롤을 확인하기 위한 한글 문장이며 줄바꿈도 함께 확인함. English words are mixed in to check wrapping.</p>", i)
	}
	b.WriteString("</article></body></html>")
	return b.String()
}

// ---- 주소 ----

func TestBrNormalizeURL(t *testing.T) {
	ok := map[string]string{
		"example.com":                "https://example.com",
		"  example.com/a/b?x=1  ":    "https://example.com/a/b?x=1",
		"http://example.com/":        "http://example.com/",
		"HTTPS://Example.com/x":      "https://Example.com/x",
		"https://example.com/p#frag": "https://example.com/p",
		"localhost:8080":             "https://localhost:8080",
		"127.0.0.1:3000/path":        "https://127.0.0.1:3000/path",
		"//example.com/x":            "https://example.com/x",
		"https://한글.example.com/가나다": "https://xn--bj0bj06e.example.com/%EA%B0%80%EB%82%98%EB%8B%A4",
	}
	for in, want := range ok {
		got, err := brNormalizeURL(in)
		if err != nil || got != want {
			t.Errorf("brNormalizeURL(%q) = %q, %v / 기대 %q", in, got, err, want)
		}
	}
	bad := []string{
		"", "   ", "file:///c:/windows/win.ini", "javascript:alert(1)", "JavaScript:alert(1)", "data:text/html,<b>x</b>",
		"ftp://example.com/x", "mailto:a@b.c", "view-source:https://example.com", "about:blank",
		"https://user:pw@example.com/", "https://user@example.com/", "http://", "https://", "exa mple.com",
		"https://example.com/\x1b[31m", "https://example.com/\u202e", strings.Repeat("a", brMaxURL+1),
		"chrome://settings", "http:example.com",
	}
	for _, in := range bad {
		if got, err := brNormalizeURL(in); err == nil {
			t.Errorf("brNormalizeURL(%q) = %q, 오류여야 함", in, got)
		}
	}
	if _, err := brNormalizeURL("file:///etc/passwd"); err == nil || !strings.Contains(err.Error(), "http") {
		t.Errorf("file: 거절 문구에 안내가 없음: %v", err)
	}
}

func TestBrResolveHref(t *testing.T) {
	base, _ := url.Parse("https://blog.example.com/posts/2026/a.html?x=1")
	cases := map[string]string{
		"b.html":                     "https://blog.example.com/posts/2026/b.html",
		"/about":                     "https://blog.example.com/about",
		"../c":                       "https://blog.example.com/posts/c",
		"//cdn.example.org/x":        "https://cdn.example.org/x",
		"https://other.org/p#top":    "https://other.org/p",
		"  http://other.org/z \n":    "http://other.org/z",
		"?page=2":                    "https://blog.example.com/posts/2026/a.html?page=2",
		"https://a.org/ja\nvascript": "https://a.org/javascript",
	}
	for href, want := range cases {
		if got, ok := brResolveHref(base, href); !ok || got != want {
			t.Errorf("brResolveHref(%q) = %q, %v / 기대 %q", href, got, ok, want)
		}
	}
	for _, href := range []string{"", "#top", "javascript:alert(1)", "mailto:a@b.c", "tel:123", "data:text/plain,hi", "file:///x", "ftp://x.org/a", "https://u:p@x.org/"} {
		if got, ok := brResolveHref(base, href); ok {
			t.Errorf("brResolveHref(%q) = %q, 거절해야 함", href, got)
		}
	}
}

func TestBrSameSite(t *testing.T) {
	if !brSameSite("https://example.com/a", "http://www.example.com:8080/b") {
		t.Error("www 와 포트는 같은 사이트로 봐야 함")
	}
	if brSameSite("https://example.com/a", "https://example.org/a") || brSameSite("https://a.example.com", "https://example.com") {
		t.Error("호스트가 다르면 다른 사이트")
	}
}

// ---- 글자 정리 (보안) ----

func TestBrCleanText(t *testing.T) {
	in := "a\x1b]0;제목바꿈\x07b\x1b[31mred\x1b[0m c\u009b31m d\r\ne\tf\x00g\x7fh\u202edcba\u202c\u2066x\u2069 \u200bzw\ufeffj\u00adk \xff\xfe l\ue000m"
	got := brClean(in)
	for _, r := range got {
		if r < 0x20 || (r >= 0x7f && r <= 0x9f) || unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Co, r) || r == brRuneError() {
			t.Errorf("정리 뒤에도 남은 글자 %U: %q", r, got)
		}
	}
	brMustContain(t, "brClean", got, "]0;제목바꿈", "[31mred", "dcba", "zwj", "jk", "e f")
	if brClean("평범한 글 ABC 123 !") != "평범한 글 ABC 123 !" {
		t.Error("평범한 글은 그대로여야 함")
	}
	pre := brCleanPre("a\tb\r\nc\x1b[2Jd\rz")
	if pre != "a    b\nc[2Jd\nz" {
		t.Errorf("brCleanPre = %q", pre)
	}
	long := brCleanPre(strings.Repeat("x", brMaxPreLine*3))
	if n := len([]rune(long)); n != brMaxPreLine+1 {
		t.Errorf("pre 한 줄 상한: %d", n)
	}
}

func brRuneError() rune { return '\uFFFD' }

const brInjectionDoc = `<html><head><title>제목 &#27;[2J 가짜</title></head><body><article>
<h1>헤드 &#x1b;]0;pwned&#7; 라인</h1>
<p>본문 <a href="https://other.example.org/&#x1b;[31mx">링크&#27;[1;31m텍스트</a> 끝 &#x202e;뒤집힘 &#x9b;31m &#155;</p>
<pre>pre&#27;[2J
	탭줄&#7;</pre>
<img alt="그림&#27;]0;img-pwned&#7;" src="x.png">
<p>` + "\x1b]0;raw-pwned\x07\x1b[31m빨강\x1b[0m\u202e\r" + `</p></article></body></html>`

func TestBrInjectionStripped(t *testing.T) {
	p := brParse(t, brInjectionDoc, "https://example.com/a")
	for _, w := range []int{46, 68} {
		for i, l := range brLayoutPage(p, w) {
			for _, s := range l {
				for _, r := range s.t {
					if r < 0x20 || (r >= 0x7f && r <= 0x9f) || unicode.Is(unicode.Cf, r) {
						t.Fatalf("폭 %d, %d번째 줄에 제어 문자 %U: %q", w, i, r, s.t)
					}
				}
			}
		}
	}
	for _, r := range p.Title {
		if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
			t.Errorf("제목에 제어 문자 %U", r)
		}
	}
	for _, l := range p.Links {
		if strings.ContainsAny(l, "\x1b\x07") {
			t.Errorf("링크 주소에 제어 문자: %q", l)
		}
	}
	txt := brText(p, 60)
	brMustContain(t, "injection", txt, "]0;pwned", "[2J", "]0;img-pwned", "]0;raw-pwned", "빨강")
}

func TestBrInjectionNeverReachesView(t *testing.T) {
	web := &brFakeWeb{pages: map[string]string{"https://example.com/a": brInjectionDoc}}
	brWithFetch(t, web.fetch)
	a := newBrowserApp()
	brOpen(t, a, "example.com/a")
	if a.cur == nil {
		t.Fatalf("페이지가 열리지 않음: %q", a.msg)
	}
	for _, sz := range testSizes {
		c := testCtx(sz[0], sz[1])
		v := a.View(c)
		assertFrame(t, "inject", c, v)
		s := plain(v)
		// ESC 가 남아 있었다면 ansi.Strip 이 뒤따르는 글자와 함께 지웠을 것이므로, 글자가 그대로 보이면 걸러졌다는 뜻
		for _, r := range s {
			if r != '\n' && (r < 0x20 || (r >= 0x7f && r <= 0x9f)) {
				t.Fatalf("화면에 제어 문자 %U", r)
			}
		}
		if sz[1] >= 24 {
			brMustContain(t, "inject-view", s, "]0;pwned")
		}
	}
	// 제어 문자가 든 링크 주소는 링크로 만들지 않음 (주소 파서가 거절함)
	if len(a.cur.Links) != 0 {
		t.Errorf("제어 문자가 든 링크가 번호를 받음: %q", a.cur.Links)
	}
}

// ---- 본문 추출 ----

const brArticleDoc = `<!doctype html><html lang="ko"><head><meta charset="utf-8"><title>테스트 글 - 내 블로그</title>
<style>.x{color:red}</style><script>var secret="스크립트내용";</script></head>
<body>
<header class="site-header"><a href="/">사이트이름헤더</a></header>
<nav><ul><li><a href="/a">메뉴하나</a></li><li><a href="/b">메뉴둘</a></li></ul></nav>
<div class="sidebar">사이드바글</div>
<article>
<h1>글 제목입니다</h1>
<p>첫 문단입니다. <a href="/next">다음 글</a> 과 <a href="https://other.example.org/x">외부 링크</a> 가 있음. 같은 링크 <a href="/next">또 다음</a>.</p>
<ul><li>사과</li><li>배 <ul><li>신배</li></ul></li></ul>
<ol start="3"><li>셋</li><li>넷</li></ol>
<blockquote><p>인용문입니다</p></blockquote>
<pre>func main() {
	fmt.Println("hi")      // 들여쓰기 유지
}</pre>
<p>그림: <img src="a.png" alt="고양이 사진"> 그리고 <img src="b.png"></p>
<p><a href="/img"><img src="c.png" alt="링크된 그림"></a></p>
<noscript>자바스크립트 필요</noscript>
<form><input type="text" value="입력값"><button>제출버튼</button><textarea>텍스트영역</textarea></form>
<footer>글 바닥글</footer>
</article>
<aside>광고 영역</aside>
<footer>사이트 푸터</footer>
<script>alert('x')</script>
</body></html>`

func TestBrExtractArticle(t *testing.T) {
	p := brParse(t, brArticleDoc, "https://blog.example.com/posts/1")
	if p.Title != "테스트 글 - 내 블로그" {
		t.Errorf("제목 = %q", p.Title)
	}
	txt := brText(p, 60)
	brMustContain(t, "article", txt,
		"# 글 제목입니다", "첫 문단입니다.", "다음 글[1]", "외부 링크[2]", "또 다음[1]",
		"• 사과", "• 배", "◦ 신배", "3. 셋", "4. 넷", "│ 인용문입니다",
		"[이미지: 고양이 사진]", "[이미지]", "[이미지: 링크된 그림][3]",
		"링크 목록", "[1] https://blog.example.com/next", "[2] https://other.example.org/x", "[3] https://blog.example.com/img")
	brMustContain(t, "pre", txt, "  func main() {", "      fmt.Println(\"hi\")")
	brMustNotContain(t, "article", txt,
		"사이트이름헤더", "메뉴하나", "사이드바글", "스크립트내용", "자바스크립트 필요", "입력값", "제출버튼", "텍스트영역", "글 바닥글", "광고 영역", "사이트 푸터", "alert")
	if len(p.Links) != 3 {
		t.Errorf("링크 수 = %d, 기대 3 (같은 주소는 한 번호): %v", len(p.Links), p.Links)
	}
}

func TestBrPreKeepsLinesAndHardWraps(t *testing.T) {
	doc := "<html><body><article><p>" + strings.Repeat("앞 문단입니다. ", 12) + "</p><pre>a\n  b\n\n" + strings.Repeat("0123456789", 12) + "\n가나다라마바사아자차카타파하" + "</pre></article></body></html>"
	p := brParse(t, doc, "https://e.com/")
	for _, w := range []int{46, 30} {
		txt := brText(p, w)
		lines := strings.Split(txt, "\n")
		for i, l := range lines {
			if lipgloss.Width(l) > w {
				t.Errorf("폭 %d 초과 (%d번째 줄): %q", w, i, l)
			}
		}
		brMustContain(t, "pre", txt, "  a", "    b")
		// 긴 줄은 잘리지 않고 이어서 나와야 함
		joined := strings.ReplaceAll(strings.ReplaceAll(txt, "\n", ""), " ", "")
		brMustContain(t, "pre-hard", joined, strings.Repeat("0123456789", 12), "가나다라마바사아자차카타파하")
	}
}

const brBodyDoc = `<html><head><title>본문 없는 구조</title></head><body>
<header><a href="/">사이트 머리글</a></header>
<nav>상단 메뉴</nav>
<div id="content"><h2>소제목</h2><p>본문 문단 하나입니다. 충분히 길게 써서 본문으로 인정받게 합니다. 한글과 english 가 섞여 있음.</p>
<p>두 번째 문단입니다. <b>굵은 글</b> 과 <code>코드</code> 도 있음.</p><br>줄바꿈 뒤 글<br>또 한 줄</div>
<div class="sidebar"><p>사이드바 위젯 글</p></div>
<div class="comments"><p>댓글 영역 글</p></div>
<div class="cookie">쿠키 안내</div>
<table><tr><th>이름</th><th>값</th></tr><tr><td>가</td><td>1</td></tr></table>
<footer>푸터 글</footer></body></html>`

func TestBrExtractBodyFallback(t *testing.T) {
	p := brParse(t, brBodyDoc, "https://e.com/x")
	txt := brText(p, 60)
	brMustContain(t, "body", txt, "## 소제목", "본문 문단 하나입니다.", "굵은 글", "코드", "줄바꿈 뒤 글", "또 한 줄", "이름 │ 값", "가 │ 1")
	brMustNotContain(t, "body", txt, "사이트 머리글", "상단 메뉴", "사이드바 위젯", "댓글 영역", "쿠키 안내", "푸터 글")
	// 줄바꿈(br)은 줄을 나눔
	if !strings.Contains(txt, "줄바꿈 뒤 글\n또 한 줄") {
		t.Errorf("br 이 줄바꿈이 되지 않음:\n%s", txt)
	}
}

func TestBrFallbackWhenChromeEatsEverything(t *testing.T) {
	// 모든 내용이 sidebar/nav 로 보이는 구조여도 빈 화면을 내지 않음
	doc := `<html><head><title>t</title></head><body><div class="sidebar"><p>` + strings.Repeat("전부 사이드바 안에 있는 글. ", 10) + `</p></div></body></html>`
	p := brParse(t, doc, "https://e.com/")
	brMustContain(t, "fallback", brText(p, 60), "전부 사이드바 안에 있는 글.")
}

func TestBrShortArticleFallsBackToBody(t *testing.T) {
	doc := `<html><head><title>t</title></head><body><article><p>짧음</p></article><div><p>` + strings.Repeat("본문은 이쪽에 있는 긴 글입니다. ", 8) + `</p></div></body></html>`
	p := brParse(t, doc, "https://e.com/")
	brMustContain(t, "short-article", brText(p, 60), "본문은 이쪽에 있는 긴 글입니다.")
}

func TestBrTitleFallbacks(t *testing.T) {
	p := brParse(t, `<html><body><h1>첫 제목</h1><p>`+strings.Repeat("글 ", 50)+`</p></body></html>`, "https://e.com/")
	if p.Title != "첫 제목" {
		t.Errorf("h1 제목 = %q", p.Title)
	}
	p = brParse(t, `<html><body><p>`+strings.Repeat("글 ", 50)+`</p></body></html>`, "https://news.example.com/x")
	if p.Title != "news.example.com" {
		t.Errorf("호스트 제목 = %q", p.Title)
	}
}

func TestBrBaseHref(t *testing.T) {
	doc := `<html><head><base href="https://cdn.example.net/dir/"></head><body><article><p>` + strings.Repeat("글 ", 80) + `<a href="rel.html">상대</a></p></article></body></html>`
	p := brParse(t, doc, "https://e.com/a/b")
	if len(p.Links) != 1 || p.Links[0] != "https://cdn.example.net/dir/rel.html" {
		t.Errorf("base 적용 안 됨: %v", p.Links)
	}
}

func TestBrHiddenAndDeepNesting(t *testing.T) {
	doc := `<html><body><article><p hidden>숨김속성</p><p style="display: none">숨김스타일</p><p>보이는 글 ` + strings.Repeat("가", 150) + `</p></article></body></html>`
	txt := brText(brParse(t, doc, "https://e.com/"), 60)
	brMustContain(t, "hidden", txt, "보이는 글")
	brMustNotContain(t, "hidden", txt, "숨김속성", "숨김스타일")
	// 파서가 겹침 512 단계를 넘는 문서를 거절함: 죽지 않고 안내만 보여야 함
	deep := strings.Repeat("<div>", 50000) + "깊은 글" + strings.Repeat("</div>", 50000)
	p := brParse(t, "<html><body><p>앞 글 "+strings.Repeat("가", 100)+"</p>"+deep+"</body></html>", "https://e.com/")
	if p.Title == "" || len(brText(p, 60)) == 0 {
		t.Error("너무 깊은 문서에서 안내가 없음")
	}
	// 512 단계 아래의 깊은 문서는 읽힘
	ok := strings.Repeat("<div>", 300) + "<p>깊은 글 " + strings.Repeat("가", 100) + "</p>" + strings.Repeat("</div>", 300)
	brMustContain(t, "deep-ok", brText(brParse(t, "<html><body>"+ok+"</body></html>", "https://e.com/"), 60), "깊은 글")
}

// ---- 줄바꿈 ----

func TestBrWrapWidths(t *testing.T) {
	samples := []string{
		"한글과 English가 섞인 문장입니다. The quick brown fox jumps over the lazy dog 그리고 또 한글 문장이 이어짐.",
		strings.Repeat("가나다라마바사아자차카타파하", 10),
		strings.Repeat("abcdefghij", 30),
		"https://example.com/" + strings.Repeat("very-long-path-segment/", 12) + "end",
		"짧음 a b c d e f 한 글 자 만 띄어쓰기 많이   많이   많이",
		"이모지 😀 가 😀😀 나 ① ② ㉠ ㎞ ａｂｃ ＡＢＣ 한자 漢字文章 かな カナ",
		"a",
	}
	for _, w := range []int{1, 2, 3, 8, 20, 36, 46, 68} {
		for si, s := range samples {
			for i, atoms := range brWrapRuns([]brRun{{t: s, st: bsText}}, w) {
				var b strings.Builder
				for _, a := range atoms {
					b.WriteString(a.t)
				}
				// 너비 1 에서 2칸짜리 글자는 어쩔 수 없이 넘으므로 제외
				if lw := lipgloss.Width(b.String()); lw > max(w, 2) {
					t.Errorf("폭 %d, 샘플 %d, %d번째 줄 폭 %d: %q", w, si, i, lw, b.String())
				}
			}
		}
	}
}

func TestBrWrapKeepsAllText(t *testing.T) {
	s := "한글과 English가 섞인 문장입니다. The quick brown fox 그리고 또 한글 문장이 이어짐. " + strings.Repeat("abcdefghij", 8)
	want := strings.Join(strings.Fields(s), "")
	for _, w := range []int{10, 24, 46} {
		var got strings.Builder
		for _, atoms := range brWrapRuns([]brRun{{t: s, st: bsText}}, w) {
			for _, a := range atoms {
				got.WriteString(a.t)
			}
		}
		if g := strings.ReplaceAll(got.String(), " ", ""); g != want {
			t.Errorf("폭 %d: 글자가 빠지거나 바뀜\n%s\n%s", w, g, want)
		}
	}
}

func TestBrWrapBreakRules(t *testing.T) {
	lines := brWrapRuns([]brRun{{t: "hello world foo", st: bsText}}, 11)
	if len(lines) != 2 || lines[0][0].t != "hello world" || lines[1][0].t != "foo" {
		t.Errorf("영문은 공백에서 나눠야 함: %v", lines)
	}
	lines = brWrapRuns([]brRun{{t: "가나다라마바", st: bsText}}, 8) // 8칸 = 4글자
	if len(lines) != 2 || lines[0][0].t != "가나다라" || lines[1][0].t != "마바" {
		t.Errorf("한글은 글자 사이에서 나눠야 함: %v", lines)
	}
	lines = brWrapRuns([]brRun{{t: "링크", st: bsLink}, {t: "[12]", st: bsNum}}, 4)
	if len(lines) < 1 {
		t.Fatal("줄이 없음")
	}
}

func TestBrLayoutAllLinesFit(t *testing.T) {
	docs := []string{brArticleDoc, brBodyDoc, brInjectionDoc, brLongDoc(12)}
	for di, d := range docs {
		p := brParse(t, d, "https://blog.example.com/posts/1")
		for _, w := range []int{46, 52, 68} {
			for i, l := range brLayoutPage(p, w) {
				if lw := segsWidth(l); lw > w {
					t.Errorf("문서 %d 폭 %d, %d번째 줄 폭 %d: %q", di, w, i, lw, brLineText(l))
				}
				if lw := lipgloss.Width(brLineText(l)); lw > w {
					t.Errorf("문서 %d 폭 %d, %d번째 줄(lipgloss) 폭 %d", di, w, i, lw)
				}
			}
		}
	}
}

func TestBrDeepListsStayInsideWidth(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 30; i++ {
		b.WriteString("<ul><li>")
		b.WriteString("<blockquote>")
	}
	b.WriteString("깊은 목록 안의 글 " + strings.Repeat("가나다 ", 30))
	p := brParse(t, "<html><body><article><p>"+strings.Repeat("글 ", 80)+"</p>"+b.String()+"</article></body></html>", "https://e.com/")
	for i, l := range brLayoutPage(p, 46) {
		if lw := segsWidth(l); lw > 46 {
			t.Errorf("%d번째 줄 폭 %d", i, lw)
		}
	}
}

// ---- 글자 코드 / 내려받기 (httptest) ----

func brServe(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	return s
}

func TestBrEUCKR(t *testing.T) {
	body := `<html><head><title>한글 제목 똠방각하</title></head><body><article><p>안녕하세요 똠방각하 EUC-KR 본문입니다. ` + strings.Repeat("가나다 ", 40) + `</p></article></body></html>`
	enc, err := korean.EUCKR.NewEncoder().String(body)
	if err != nil {
		t.Fatalf("픽스처 인코딩 실패: %v", err)
	}
	if strings.Contains(enc, "안녕") {
		t.Fatal("픽스처가 UTF-8 로 남아 있음")
	}
	cases := map[string]string{
		"헤더 charset": "text/html; charset=euc-kr",
		"헤더 ks_c":    "text/html; charset=ks_c_5601-1987",
		"헤더 cp949":   "text/html; charset=CP949",
		"meta 만":     "text/html",
	}
	for name, ct := range cases {
		doc := enc
		if name == "meta 만" {
			doc = strings.Replace(enc, "<head>", `<head><meta http-equiv="Content-Type" content="text/html; charset=euc-kr">`, 1)
		}
		s := brServe(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", ct)
			w.Write([]byte(doc))
		})
		p, err := brFetchPage(context.Background(), s.URL)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !strings.Contains(p.Title, "한글 제목 똠방각하") {
			t.Errorf("%s: 제목 = %q", name, p.Title)
		}
		brMustContain(t, name, brText(p, 60), "안녕하세요", "똠방각하", "EUC-KR 본문입니다.")
	}
}

func TestBrUTF8WithoutDeclaration(t *testing.T) {
	s := brServe(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html") // charset 표시 없음
		w.Write([]byte("<html><body><article><p>선언 없는 UTF-8 문서 " + strings.Repeat("가나다 ", 60) + "</p></article></body></html>"))
	})
	p, err := brFetchPage(context.Background(), s.URL)
	if err != nil {
		t.Fatal(err)
	}
	brMustContain(t, "utf8", brText(p, 60), "선언 없는 UTF-8 문서")
}

func TestBrFetchHeadersNoCookiesNoCreds(t *testing.T) {
	var mu sync.Mutex
	var hdrs []http.Header
	var s *httptest.Server
	s = brServe(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hdrs = append(hdrs, r.Header.Clone())
		mu.Unlock()
		if r.URL.Path == "/" {
			http.SetCookie(w, &http.Cookie{Name: "sid", Value: "secret"})
			http.Redirect(w, r, "/two", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<html><body><p>"+strings.Repeat("글 ", 60)+"</p></body></html>")
	})
	if _, err := brFetchPage(context.Background(), s.URL); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(hdrs) != 2 {
		t.Fatalf("요청 수 %d", len(hdrs))
	}
	for i, h := range hdrs {
		if h.Get("Cookie") != "" || h.Get("Authorization") != "" || h.Get("Referer") != "" {
			t.Errorf("요청 %d 에 쿠키/인증/리퍼러가 실림: %v", i, h)
		}
		ua := h.Get("User-Agent")
		if !strings.Contains(ua, "TUI-playground") || strings.Contains(ua, "Mozilla") {
			t.Errorf("User-Agent = %q", ua)
		}
	}
}

func TestBrRedirects(t *testing.T) {
	hop := func(n int) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			var i int
			fmt.Sscanf(r.URL.Path, "/r/%d", &i)
			if i < n {
				http.Redirect(w, r, fmt.Sprintf("/r/%d", i+1), http.StatusMovedPermanently)
				return
			}
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, "<html><head><title>도착</title></head><body><p>끝 "+strings.Repeat("글 ", 50)+"</p></body></html>")
		}
	}
	s4 := brServe(t, hop(4))
	p, err := brFetchPage(context.Background(), s4.URL+"/r/0")
	if err != nil {
		t.Fatalf("리다이렉트 4번은 허용: %v", err)
	}
	if !strings.HasSuffix(p.URL, "/r/4") || p.Title != "도착" {
		t.Errorf("최종 주소 = %s, 제목 %q", p.URL, p.Title)
	}
	s5 := brServe(t, hop(5))
	if _, err := brFetchPage(context.Background(), s5.URL+"/r/0"); err == nil || !strings.Contains(err.Error(), "리다이렉트") {
		t.Errorf("리다이렉트 5번은 거절: %v", err)
	}
}

func TestBrRedirectToBadSchemeRefused(t *testing.T) {
	s := brServe(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "file:///c:/windows/win.ini", http.StatusFound)
	})
	if _, err := brFetchPage(context.Background(), s.URL); err == nil {
		t.Error("file: 로의 리다이렉트를 따라가면 안 됨")
	}
	s2 := brServe(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "http://user:pw@127.0.0.1:1/")
		w.WriteHeader(http.StatusFound)
	})
	if _, err := brFetchPage(context.Background(), s2.URL); err == nil {
		t.Error("계정 정보가 든 주소로의 리다이렉트를 따라가면 안 됨")
	}
}

func TestBrTimeout(t *testing.T) {
	brWithTimeout(t, 200*time.Millisecond)
	s := brServe(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	})
	start := time.Now()
	_, err := brFetchPage(context.Background(), s.URL)
	if err == nil || !strings.Contains(err.Error(), "시간 초과") {
		t.Errorf("시간 초과 오류여야 함: %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Errorf("너무 오래 걸림: %v", time.Since(start))
	}
}

func TestBrSlowBodyTimesOut(t *testing.T) {
	brWithTimeout(t, 300*time.Millisecond)
	s := brServe(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><body><p>천천히"))
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	})
	if _, err := brFetchPage(context.Background(), s.URL); err == nil || !strings.Contains(err.Error(), "시간 초과") {
		t.Errorf("본문이 느려도 시간 초과여야 함: %v", err)
	}
}

func TestBrStatusErrors(t *testing.T) {
	for _, code := range []int{400, 403, 404, 410, 500, 503} {
		s := brServe(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(code)
			fmt.Fprint(w, "<html><body>오류 본문 \x1b[31m</body></html>")
		})
		_, err := brFetchPage(context.Background(), s.URL)
		want := fmt.Sprintf("HTTP %d %s", code, http.StatusText(code))
		if err == nil || err.Error() != want {
			t.Errorf("상태 %d: %v / 기대 %q", code, err, want)
		}
	}
	s := brServe(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotModified) })
	if _, err := brFetchPage(context.Background(), s.URL); err == nil {
		t.Error("304 는 오류로 알려야 함")
	}
}

func TestBrNonHTML(t *testing.T) {
	for _, ct := range []string{"image/png", "application/pdf", "application/json", "application/octet-stream", "video/mp4"} {
		var served bool
		s := brServe(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", ct)
			w.Write([]byte("\x89PNG\r\n\x1a\n\x00\x00binary"))
			served = true
		})
		_, err := brFetchPage(context.Background(), s.URL)
		if err == nil || !strings.Contains(err.Error(), ct) || !strings.Contains(err.Error(), "표시하지 않음") {
			t.Errorf("%s: 형식을 알리는 오류여야 함: %v", ct, err)
		}
		_ = served
	}
	// 형식 헤더가 없으면 내용을 보고 판단함
	s := brServe(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header()["Content-Type"] = nil
		w.Write([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"))
	})
	if _, err := brFetchPage(context.Background(), s.URL); err == nil || !strings.Contains(err.Error(), "image/png") {
		t.Errorf("내용으로 PNG 를 알아봐야 함: %v", err)
	}
}

func TestBrPlainText(t *testing.T) {
	s := brServe(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte("첫 줄\n\t들여쓴 줄 \x1b[31m빨강\n마지막"))
	})
	p, err := brFetchPage(context.Background(), s.URL)
	if err != nil {
		t.Fatal(err)
	}
	txt := brText(p, 60)
	brMustContain(t, "plain", txt, "첫 줄", "들여쓴 줄 [31m빨강", "마지막")
	if strings.ContainsRune(txt, 0x1b) {
		t.Error("ESC 가 남음")
	}
}

func TestBrTruncatedBody(t *testing.T) {
	s := brServe(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<html><body><article><p>시작 부분 "+strings.Repeat("가", 200)+"</p>")
		chunk := strings.Repeat("<p>x</p>", 1000)
		for n := 0; n < brMaxBody+2000; n += len(chunk) {
			fmt.Fprint(w, chunk)
		}
		fmt.Fprint(w, "<p>끝부분표시</p></article></body></html>")
	})
	p, err := brFetchPage(context.Background(), s.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Truncated {
		t.Error("2MB 를 넘으면 Truncated 여야 함")
	}
	brMustContain(t, "trunc", brText(p, 60), "시작 부분")
	brMustNotContain(t, "trunc", brText(p, 60), "끝부분표시")

	// 화면에도 잘렸다는 안내가 보여야 함
	brWithFetch(t, func(ctx context.Context, raw string) (*brPage, error) { return p, nil })
	a := newBrowserApp()
	brOpen(t, a, "example.com/t")
	if !strings.Contains(plain(a.View(testCtx(80, 24))), "2MB") || a.msgTone != ToneWarn {
		t.Errorf("잘렸다는 안내가 없음: %q", a.msg)
	}
}

func TestBrBigDocumentIsFast(t *testing.T) {
	var b strings.Builder
	b.WriteString("<html><head><title>큰 글</title></head><body><article>")
	for b.Len() < 1500000 {
		b.WriteString("<p>이것은 아주 긴 한글 문서의 한 문단입니다 with some English words <a href=\"/x\">링크</a> 그리고 계속 이어짐.</p>")
	}
	b.WriteString("</article></body></html>")
	start := time.Now()
	p := brParse(t, b.String(), "https://big.example.com/")
	parse := time.Since(start)
	start = time.Now()
	lines := brLayoutPage(p, 68)
	lay := time.Since(start)
	t.Logf("1.5MB 문서: 정리 %v, 줄바꿈 %v, %d줄, 링크 %d", parse, lay, len(lines), len(p.Links))
	if parse+lay > 10*time.Second {
		t.Errorf("너무 느림: %v", parse+lay)
	}
}

// 네트워크 오류 분류는 가짜 오류로 확인함 (진짜 DNS 조회를 하지 않음)
func TestBrClassifyErrors(t *testing.T) {
	ctx := context.Background()
	dns := &url.Error{Op: "Get", URL: "https://x.invalid/", Err: &net.DNSError{Name: "x.invalid\x1b[31m", IsNotFound: true}}
	err := brClassifyErr(ctx, dns)
	if !strings.Contains(err.Error(), "주소를 찾을 수 없음") || !strings.Contains(err.Error(), "x.invalid") || strings.ContainsRune(err.Error(), 0x1b) {
		t.Errorf("DNS 오류 문구 = %q", err)
	}
	to := &url.Error{Op: "Get", URL: "https://x.invalid/", Err: &net.OpError{Op: "dial", Err: timeoutErr{}}}
	if err := brClassifyErr(ctx, to); !strings.Contains(err.Error(), "시간 초과") {
		t.Errorf("시간 초과 문구 = %q", err)
	}
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if err := brClassifyErr(cctx, context.Canceled); err != errBrCanceled {
		t.Errorf("취소 = %v", err)
	}
	if err := brClassifyErr(ctx, &url.Error{Op: "Get", URL: "x", Err: errors.New("connection refused \x1b]0;x\x07")}); !strings.Contains(err.Error(), "연결 실패") || strings.ContainsAny(err.Error(), "\x1b\x07") {
		t.Errorf("일반 오류 문구 = %q", err)
	}
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestBrRejectsBadSchemeInFetch(t *testing.T) {
	for _, in := range []string{"file:///etc/passwd", "javascript:alert(1)", "ftp://x.org"} {
		if _, err := brFetchPage(context.Background(), in); err == nil {
			t.Errorf("%q 를 열면 안 됨", in)
		}
	}
}

// 취소하면 요청이 끝나고 고루틴이 남지 않음
func TestBrCancelNoLeak(t *testing.T) {
	released := make(chan struct{})
	s := brServe(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(10 * time.Second):
		}
		close(released)
	})
	before := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := brFetchPage(ctx, s.URL); done <- err }()
	time.Sleep(150 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != errBrCanceled {
			t.Errorf("취소 오류여야 함: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("취소했는데 brFetchPage 가 끝나지 않음")
	}
	select {
	case <-released:
	case <-time.After(3 * time.Second):
		t.Fatal("서버 핸들러가 정리되지 않음(연결이 닫히지 않음)")
	}
	deadline := time.Now().Add(3 * time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > before {
		buf := make([]byte, 1<<16)
		t.Errorf("고루틴 %d → %d, 남은 것이 있음:\n%s", before, n, buf[:runtime.Stack(buf, true)])
	}
}

// ---- 앱 흐름 ----

func TestBrFreshEscGoesHome(t *testing.T) {
	a := newBrowserApp()
	_, cmd := a.Update(key("esc"))
	if cmd == nil {
		t.Fatal("명령이 없음")
	}
	if _, ok := cmd().(goHomeMsg); !ok {
		t.Error("goHomeMsg 가 아님")
	}
}

func TestBrInputRejectsBadScheme(t *testing.T) {
	web := &brFakeWeb{pages: map[string]string{}}
	brWithFetch(t, web.fetch)
	a := newBrowserApp()
	for _, in := range []string{"file:///c:/x", "javascript:alert(1)", "data:text/html,hi", "ftp://a.b/c"} {
		a.input.SetValue(in)
		_, cmd := a.Update(key("enter"))
		if cmd != nil || a.loading {
			t.Errorf("%q: 요청이 시작됨", in)
		}
		if a.msgTone != ToneDanger || !strings.Contains(a.msg, "http") {
			t.Errorf("%q: 거절 문구 = %q", in, a.msg)
		}
	}
	if len(web.fetched()) != 0 {
		t.Errorf("요청이 나감: %v", web.fetched())
	}
	a.input.SetValue("")
	a.Update(key("enter"))
	if a.msg == "" {
		t.Error("빈 입력 안내가 없음")
	}
}

func TestBrOpenAndRead(t *testing.T) {
	web := &brFakeWeb{pages: map[string]string{"https://example.com": brArticleDoc}}
	brWithFetch(t, web.fetch)
	a := newBrowserApp()
	brOpen(t, a, "example.com")
	if a.cur == nil || a.mode != brRead || a.loading {
		t.Fatalf("읽기 상태가 아님: mode=%d loading=%v msg=%q", a.mode, a.loading, a.msg)
	}
	if got := web.fetched(); len(got) != 1 || got[0] != "https://example.com" {
		t.Errorf("요청 주소 = %v", got)
	}
	v := plain(a.View(testCtx(80, 24)))
	brMustContain(t, "view", v, "테스트 글 - 내 블로그", "https://example.com", "글 제목입니다", "[Esc] 홈으로")
}

func TestBrLoadingStateAndEscCancel(t *testing.T) {
	started := make(chan struct{})
	finished := make(chan error, 1)
	var ctxSeen context.Context
	brWithFetch(t, func(ctx context.Context, raw string) (*brPage, error) {
		ctxSeen = ctx
		close(started)
		<-ctx.Done()
		finished <- ctx.Err()
		return nil, errBrCanceled
	})
	a := newBrowserApp()
	a.input.SetValue("example.com")
	_, cmd := a.Update(key("enter"))
	if !a.loading || cmd == nil {
		t.Fatal("불러오기가 시작되지 않음")
	}
	msgs := make(chan tea.Msg, 4)
	b := cmd().(tea.BatchMsg)
	for _, c := range b {
		go func(c tea.Cmd) { msgs <- c() }(c)
	}
	<-started
	c := testCtx(80, 24)
	c.Now = a.started.Add(3 * time.Second)
	v := plain(a.View(c))
	brMustContain(t, "loading", v, "불러오는 중", "3초", "[Esc] 취소하고 홈으로")
	// 틱: 같은 번호면 이어지고, 다른 번호나 불러오는 중이 아니면 멈춤
	if _, cmd := a.Update(appTickMsg{Gen: a.gen, Tag: "load"}); cmd == nil {
		t.Error("불러오는 중에는 틱이 이어져야 함")
	}
	if _, cmd := a.Update(appTickMsg{Gen: a.gen + 999, Tag: "load"}); cmd != nil {
		t.Error("다른 번호의 틱은 무시해야 함")
	}
	// 다른 키는 무시, Esc 는 요청을 취소하고 홈으로
	a.Update(key("j"))
	if !a.loading {
		t.Fatal("다른 키로 취소되면 안 됨")
	}
	_, cmd = a.Update(key("esc"))
	if a.loading || cmd == nil {
		t.Fatal("Esc 는 요청을 취소하고 홈으로 가야 함")
	}
	if _, ok := cmd().(goHomeMsg); !ok {
		t.Error("goHomeMsg 가 아님")
	}
	select {
	case err := <-finished:
		if err == nil || ctxSeen.Err() == nil {
			t.Error("요청 컨텍스트가 취소되지 않음")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("취소가 fetch 에 전달되지 않음")
	}
	// 늦게 도착한 완료 메시지는 무시
	for i := 0; i < len(b); i++ {
		if d, ok := (<-msgs).(brDoneMsg); ok {
			a.Update(d)
		}
	}
	if a.cur != nil || a.loading {
		t.Error("취소한 요청의 결과가 반영됨")
	}
	if !strings.Contains(a.msg, "취소") {
		t.Errorf("취소 안내 = %q", a.msg)
	}
	if _, tick := a.Update(appTickMsg{Gen: a.gen, Tag: "load"}); tick != nil {
		t.Error("취소 뒤에는 틱이 멈춰야 함")
	}
	// 홈에 다녀온 뒤 새로 만든 앱은 이전 요청의 늦은 완료 메시지를 무시함
	fresh := newBrowserApp()
	fresh.Update(brDoneMsg{Req: a.req, Page: &brPage{URL: "https://late.example.com", Title: "늦음"}})
	if fresh.cur != nil {
		t.Error("다른 인스턴스의 요청 결과가 반영됨")
	}
}

func TestBrFetchErrorShownAndKeepsPage(t *testing.T) {
	web := &brFakeWeb{pages: map[string]string{"https://a.com": brLongDoc(3)}}
	brWithFetch(t, web.fetch)
	a := newBrowserApp()
	brOpen(t, a, "a.com")
	first := a.cur
	brOpen(t, a, "missing.com")
	if a.cur != first || a.msgTone != ToneDanger || !strings.Contains(a.msg, "404") {
		t.Errorf("오류 뒤에도 이전 페이지가 남아야 함: cur=%v msg=%q", a.cur == first, a.msg)
	}
	if len(a.hist) != 1 {
		t.Errorf("실패한 요청이 기록에 들어감: %d", len(a.hist))
	}
	if !strings.Contains(plain(a.View(testCtx(80, 24))), "HTTP 404 Not Found") {
		t.Error("메시지 줄에 오류가 보이지 않음")
	}
}

const brLinkPage = `<html><head><title>링크 페이지</title></head><body><article><p>` + "본문 글이 충분히 길어야 본문으로 인정됨. 한글 문장을 반복해서 길이를 늘림. 한글 문장을 반복해서 길이를 늘림. 한글 문장을 반복해서 길이를 늘림. 한글 문장을 반복해서 길이를 늘림. " + `
<a href="/same">같은 사이트</a> <a href="https://www.other.org/page?q=1">다른 사이트</a></p></article></body></html>`

func TestBrFollowLinks(t *testing.T) {
	web := &brFakeWeb{pages: map[string]string{
		"https://site.com/start":         brLinkPage,
		"https://site.com/same":          brLongDoc(2),
		"https://www.other.org/page?q=1": brLongDoc(2),
	}}
	brWithFetch(t, web.fetch)
	a := newBrowserApp()
	brOpen(t, a, "site.com/start")
	c := testCtx(80, 24)

	// 같은 사이트: 번호를 치면 미리보기가 보이고 Enter 로 바로 이동
	a.Update(key("1"))
	if a.mode != brLink {
		t.Fatal("숫자를 치면 링크 입력 상태여야 함")
	}
	brMustContain(t, "preview", plain(a.View(c)), "[1] → https://site.com/same")
	_, cmd := a.Update(key("enter"))
	brDrive(t, a, cmd)
	if a.cur == nil || a.cur.Title != "긴 글" || a.pos != 1 {
		t.Fatalf("같은 사이트 링크로 이동하지 못함: %+v", a.cur)
	}

	// 다른 사이트: 미리보기에 경고, Enter 하면 확인 상태, n 이면 취소
	a.Update(key("h"))
	a.Update(key("f"))
	a.Update(key("2"))
	v := plain(a.View(c))
	brMustContain(t, "other-preview", v, "[2] 다른 사이트 → https://www.other.org/page?q=1")
	if a.msgTone != ToneInfo && a.mode != brLink {
		t.Fatal("상태 이상")
	}
	n := len(web.fetched())
	a.Update(key("enter"))
	if a.mode != brConfirm {
		t.Fatalf("다른 사이트는 확인을 거쳐야 함: mode=%d", a.mode)
	}
	if len(web.fetched()) != n {
		t.Error("확인 전에 요청이 나감")
	}
	v = plain(a.View(c))
	brMustContain(t, "confirm", v, "→ https://www.other.org/page?q=1", "다른 사이트로 이동: other.org", "[y] 이동", "[n] 취소")
	a.Update(key("n"))
	if a.mode != brRead || len(web.fetched()) != n {
		t.Error("n 은 취소여야 함")
	}
	a.Update(key("2"))
	a.Update(key("enter"))
	_, cmd = a.Update(key("y"))
	brDrive(t, a, cmd)
	if a.cur == nil || a.pos != 1 || len(a.hist) != 2 {
		t.Errorf("y 로 이동하지 못함: pos=%d hist=%d", a.pos, len(a.hist))
	}
	if got := web.fetched(); got[len(got)-1] != "https://www.other.org/page?q=1" {
		t.Errorf("마지막 요청 = %v", got)
	}
}

func TestBrFollowBadLinkNumber(t *testing.T) {
	web := &brFakeWeb{pages: map[string]string{"https://site.com/start": brLinkPage}}
	brWithFetch(t, web.fetch)
	a := newBrowserApp()
	brOpen(t, a, "site.com/start")
	a.Update(key("9"))
	a.Update(key("9"))
	brMustContain(t, "bad", plain(a.View(testCtx(80, 24))), "링크 99 번이 없음")
	_, cmd := a.Update(key("enter"))
	if cmd != nil || a.mode != brRead || a.msgTone != ToneDanger {
		t.Errorf("없는 링크: mode=%d msg=%q", a.mode, a.msg)
	}
	// Backspace 로 지우다 비면 읽기로 돌아오고, 이전 페이지로 가지는 않음
	a.Update(key("1"))
	a.Update(brKey("backspace"))
	if a.mode != brRead || a.pos != 0 {
		t.Error("Backspace 로 입력을 지우면 읽기 상태로 돌아와야 함")
	}
	// 숫자가 아닌 키는 입력을 취소함
	a.Update(key("2"))
	a.Update(key("x"))
	if a.mode != brRead {
		t.Error("숫자가 아닌 키는 링크 입력을 취소해야 함")
	}
	// Esc 는 어디서나 홈으로
	a.Update(key("1"))
	if _, cmd := a.Update(key("esc")); cmd == nil {
		t.Error("링크 입력 중에도 Esc 는 홈으로")
	}
}

func TestBrHistoryBackForward(t *testing.T) {
	web := &brFakeWeb{pages: map[string]string{
		"https://a.com": brLongDoc(40), "https://b.com": brLongDoc(5), "https://c.com": brLongDoc(5),
	}}
	brWithFetch(t, web.fetch)
	a := newBrowserApp()
	c := testCtx(80, 24)
	brOpen(t, a, "a.com")
	a.View(c)
	a.Update(key("G"))
	a.View(c)
	bottomA := a.scroll
	if bottomA == 0 {
		t.Fatal("스크롤이 움직이지 않음")
	}
	brOpen(t, a, "b.com")
	brOpen(t, a, "c.com")
	if len(a.hist) != 3 || a.pos != 2 {
		t.Fatalf("기록 %d, 위치 %d", len(a.hist), a.pos)
	}
	n := len(web.fetched())
	a.Update(brKey("backspace"))
	a.Update(key("h"))
	if a.pos != 0 || a.cur != a.hist[0].page {
		t.Fatalf("뒤로 두 번: pos=%d", a.pos)
	}
	a.View(c)
	if a.scroll != bottomA {
		t.Errorf("스크롤 위치가 복원되지 않음: %d != %d", a.scroll, bottomA)
	}
	a.Update(key("h"))
	if a.pos != 0 || !strings.Contains(a.msg, "이전") {
		t.Errorf("맨 앞에서 더 뒤로: pos=%d msg=%q", a.pos, a.msg)
	}
	a.Update(key("l"))
	a.Update(brKey("right"))
	if a.pos != 2 {
		t.Errorf("앞으로 두 번: pos=%d", a.pos)
	}
	a.Update(key("l"))
	if a.pos != 2 || !strings.Contains(a.msg, "다음") {
		t.Errorf("맨 뒤에서 더 앞으로: pos=%d msg=%q", a.pos, a.msg)
	}
	if len(web.fetched()) != n {
		t.Error("뒤로/앞으로는 다시 내려받지 않아야 함")
	}
	// 중간에서 새로 열면 앞쪽 기록이 사라짐
	a.Update(key("h"))
	brOpen(t, a, "a.com")
	if len(a.hist) != 3 || a.pos != 2 {
		t.Errorf("새로 열면 앞쪽 기록을 버려야 함: 기록 %d, 위치 %d", len(a.hist), a.pos)
	}
}

func TestBrHistoryCap(t *testing.T) {
	web := &brFakeWeb{pages: map[string]string{}}
	for i := 0; i < brMaxHist+5; i++ {
		web.pages[fmt.Sprintf("https://h%d.com", i)] = brLongDoc(1)
	}
	brWithFetch(t, web.fetch)
	a := newBrowserApp()
	for i := 0; i < brMaxHist+5; i++ {
		brOpen(t, a, fmt.Sprintf("h%d.com", i))
	}
	if len(a.hist) != brMaxHist || a.pos != brMaxHist-1 {
		t.Errorf("기록 %d, 위치 %d", len(a.hist), a.pos)
	}
}

func TestBrReload(t *testing.T) {
	web := &brFakeWeb{pages: map[string]string{"https://a.com": brLongDoc(3)}}
	brWithFetch(t, web.fetch)
	a := newBrowserApp()
	brOpen(t, a, "a.com")
	_, cmd := a.Update(key("r"))
	if !a.loading {
		t.Fatal("r 은 다시 불러와야 함")
	}
	brDrive(t, a, cmd)
	if len(a.hist) != 1 || a.pos != 0 || len(web.fetched()) != 2 {
		t.Errorf("새로고침은 기록을 늘리지 않음: 기록 %d, 요청 %v", len(a.hist), web.fetched())
	}
}

func TestBrScrollBounds(t *testing.T) {
	web := &brFakeWeb{pages: map[string]string{"https://a.com": brLongDoc(40)}}
	brWithFetch(t, web.fetch)
	a := newBrowserApp()
	brOpen(t, a, "a.com")
	for _, sz := range testSizes {
		c := testCtx(sz[0], sz[1])
		a.scroll = 0
		a.View(c)
		total := len(a.lines())
		top := total - a.vh
		if top <= 0 {
			t.Fatalf("%v: 문서가 너무 짧음", sz)
		}
		check := func(step string) {
			t.Helper()
			a.View(c)
			if a.scroll < 0 || a.scroll > top {
				t.Fatalf("%v %s: 스크롤 %d, 범위 0..%d", sz, step, a.scroll, top)
			}
		}
		a.Update(key("k"))
		check("맨 위에서 k")
		if a.scroll != 0 {
			t.Errorf("%v: 맨 위에서 k 하면 0 이어야 함", sz)
		}
		a.Update(key("j"))
		a.Update(key("down"))
		check("j, down")
		if a.scroll != 2 {
			t.Errorf("%v: j + down 뒤 %d", sz, a.scroll)
		}
		a.Update(key("up"))
		check("up")
		if a.scroll != 1 {
			t.Errorf("%v: up 뒤 %d", sz, a.scroll)
		}
		a.Update(brKey("pgdown"))
		check("pgdown")
		if want := 1 + a.vh - 1; a.scroll != want {
			t.Errorf("%v: pgdown 뒤 %d, 기대 %d", sz, a.scroll, want)
		}
		a.Update(brKey(" "))
		check("space")
		a.Update(brKey("pgup"))
		check("pgup")
		a.Update(key("G"))
		check("G")
		if a.scroll != top {
			t.Errorf("%v: G 뒤 %d, 기대 %d", sz, a.scroll, top)
		}
		a.Update(key("j"))
		a.Update(brKey("pgdown"))
		check("끝에서 아래로")
		if a.scroll != top {
			t.Errorf("%v: 끝을 넘어감 %d", sz, a.scroll)
		}
		if !strings.Contains(plain(a.View(c)), "100%") {
			t.Errorf("%v: 끝에서 100%% 가 아님", sz)
		}
		a.Update(key("g"))
		check("g")
		if a.scroll != 0 {
			t.Errorf("%v: g 뒤 %d", sz, a.scroll)
		}
		// 가끔 한 번에 크게 넘겨도 범위 안
		for i := 0; i < 500; i++ {
			a.Update(brKey("pgdown"))
		}
		check("pgdown 500번")
		for i := 0; i < 500; i++ {
			a.Update(brKey("pgup"))
		}
		check("pgup 500번")
	}
}

func TestBrShortDocumentNoScroll(t *testing.T) {
	web := &brFakeWeb{pages: map[string]string{"https://a.com": "<html><body><p>" + strings.Repeat("짧은 글 ", 30) + "</p></body></html>"}}
	brWithFetch(t, web.fetch)
	a := newBrowserApp()
	brOpen(t, a, "a.com")
	c := testCtx(120, 40)
	a.View(c)
	a.Update(key("j"))
	a.Update(key("G"))
	a.View(c)
	if a.scroll != 0 {
		t.Errorf("짧은 문서가 스크롤됨: %d", a.scroll)
	}
}

// ---- 화면 ----

func TestBrViewsAlignedAllSizesAndStates(t *testing.T) {
	web := &brFakeWeb{pages: map[string]string{
		"https://site.com/start": brLinkPage, "https://blog.example.com/posts/1": brArticleDoc,
		"https://wide.com": brLongDoc(30), "https://www.other.org/page?q=1": brLongDoc(2),
	}}
	brWithFetch(t, web.fetch)

	type state struct {
		name  string
		setup func(t *testing.T) *browserApp
	}
	states := []state{
		{"처음", func(t *testing.T) *browserApp { return newBrowserApp() }},
		{"입력 오류", func(t *testing.T) *browserApp {
			a := newBrowserApp()
			a.input.SetValue("javascript:alert(1)")
			a.Update(key("enter"))
			return a
		}},
		{"긴 입력", func(t *testing.T) *browserApp {
			a := newBrowserApp()
			a.input.SetValue("https://example.com/" + strings.Repeat("한글경로/", 40))
			a.input.CursorEnd()
			return a
		}},
		{"읽기", func(t *testing.T) *browserApp {
			a := newBrowserApp()
			brOpen(t, a, "wide.com")
			return a
		}},
		{"글 제목이 긴 페이지", func(t *testing.T) *browserApp {
			a := newBrowserApp()
			brOpen(t, a, "site.com/start")
			a.cur.Title = strings.Repeat("아주 긴 제목 ", 30)
			a.cur.URL = "https://site.com/" + strings.Repeat("긴주소/", 50)
			return a
		}},
		{"읽기 중 주소 입력", func(t *testing.T) *browserApp {
			a := newBrowserApp()
			brOpen(t, a, "wide.com")
			a.Update(key("o"))
			a.input.SetValue("example.org")
			return a
		}},
		{"링크 입력", func(t *testing.T) *browserApp {
			a := newBrowserApp()
			brOpen(t, a, "site.com/start")
			a.Update(key("2"))
			return a
		}},
		{"이동 확인", func(t *testing.T) *browserApp {
			a := newBrowserApp()
			brOpen(t, a, "site.com/start")
			a.Update(key("2"))
			a.Update(key("enter"))
			return a
		}},
		{"불러오는 중", func(t *testing.T) *browserApp {
			a := newBrowserApp()
			a.input.SetValue("example.com")
			a.Update(key("enter"))
			t.Cleanup(a.cancelLoad)
			return a
		}},
		{"오류 메시지", func(t *testing.T) *browserApp {
			a := newBrowserApp()
			brOpen(t, a, "wide.com")
			a.say("연결 실패: "+strings.Repeat("아주 긴 오류 설명 ", 20), ToneDanger)
			return a
		}},
		{"본문 없는 페이지", func(t *testing.T) *browserApp {
			a := newBrowserApp()
			a.cur = &brPage{URL: "https://empty.com", Title: "빈 문서"}
			a.hist, a.pos, a.mode = []brEntry{{page: a.cur}}, 0, brRead
			return a
		}},
	}
	for _, st := range states {
		for _, sz := range testSizes {
			a := st.setup(t)
			c := testCtx(sz[0], sz[1])
			name := fmt.Sprintf("%s %dx%d", st.name, sz[0], sz[1])
			v := a.View(c)
			assertFrame(t, name, c, v)
			lines := strings.Split(v, "\n")
			if len(lines) > c.H {
				t.Errorf("%s: 줄 수 %d 가 창 높이 %d 를 넘음", name, len(lines), c.H)
			}
			for i, l := range lines {
				if w := lipgloss.Width(l); w != c.In()+2 {
					t.Errorf("%s: %d번째 줄 폭 %d, 기대 %d: %q", name, i, w, c.In()+2, plain(l))
				}
			}
			s := plain(v)
			brMustContain(t, name, s, "[Esc]")
			if sz[0] >= 80 && st.name == "읽기" {
				brMustContain(t, name, s, "긴 글", "%")
			}
		}
	}
}

// 이모지·결합 문자는 글자 하나씩 잰 폭과 실제 폭이 달라서 오른쪽 테두리를 밀 수 있음
func TestBrEmojiWidthsKeepFrame(t *testing.T) {
	emo := "❤️✔️👍🏽é 1️⃣ 👨‍👩‍👧 🇰🇷 ☺️"
	doc := "<html><head><title>" + strings.Repeat(emo, 8) + "</title></head><body><article><h2>" + strings.Repeat(emo, 6) + "</h2><p>" +
		strings.Repeat(emo+" 한글 ", 12) + "<a href=\"/x\">" + emo + "</a></p><pre>" + strings.Repeat(emo, 30) + "\n" + strings.Repeat("👍🏽", 40) + "</pre></article></body></html>"
	brWithFetch(t, (&brFakeWeb{pages: map[string]string{"https://e.com": doc}}).fetch)
	a := newBrowserApp()
	brOpen(t, a, "e.com")
	if a.cur == nil {
		t.Fatalf("열리지 않음: %q", a.msg)
	}
	for _, sz := range testSizes {
		c := testCtx(sz[0], sz[1])
		for _, scroll := range []int{0, 3, 1000} {
			a.scroll = scroll
			v := a.View(c)
			name := fmt.Sprintf("이모지 %dx%d scroll=%d", sz[0], sz[1], scroll)
			assertFrame(t, name, c, v)
			for i, l := range strings.Split(v, "\n") {
				if w := lipgloss.Width(l); w != c.In()+2 {
					t.Errorf("%s: %d번째 줄 폭 %d, 기대 %d: %q", name, i, w, c.In()+2, plain(l))
				}
			}
		}
	}
	for _, w := range []int{46, 68} {
		for i, l := range brLayoutPage(a.cur, w) {
			if lw := lipgloss.Width(brLineText(l)); lw > w {
				t.Errorf("폭 %d, %d번째 줄 폭 %d", w, i, lw)
			}
		}
	}
}

func TestBrMinimumShowsTitleBodyHint(t *testing.T) {
	web := &brFakeWeb{pages: map[string]string{"https://wide.com": brLongDoc(30)}}
	brWithFetch(t, web.fetch)
	a := newBrowserApp()
	brOpen(t, a, "wide.com")
	c := testCtx(56, 17)
	v := plain(a.View(c))
	brMustContain(t, "min", v, "긴 글", "https://wide.com", "문단 1 번입니다", "[Esc] 홈으로")
	if n := len(strings.Split(v, "\n")); n > 17 {
		t.Errorf("줄 수 %d", n)
	}
	// 읽기 상태에서 본문 줄 수가 창 높이에 맞게 늘어남
	if a.vh != 17-10 {
		t.Errorf("본문 높이 = %d, 기대 %d", a.vh, 7)
	}
}

func TestBrRegistryEntry(t *testing.T) {
	if browserInfo.ID != "browser" || browserInfo.Name != "웹 브라우저" || browserInfo.Desc == "" || browserInfo.New == nil {
		t.Errorf("browserInfo = %+v", browserInfo)
	}
	if a := browserInfo.New(&Env{}); a == nil {
		t.Error("New 가 nil")
	}
}

func TestBrNoRealNetworkInUITests(t *testing.T) {
	// 앱이 쓰는 내려받기 함수는 변수로 바꿔 끼울 수 있어야 함
	called := false
	brWithFetch(t, func(ctx context.Context, raw string) (*brPage, error) {
		called = true
		return nil, fmt.Errorf("가짜 오류")
	})
	a := newBrowserApp()
	brOpen(t, a, "example.com")
	if !called || !strings.Contains(a.msg, "가짜 오류") {
		t.Errorf("가짜 fetch 가 쓰이지 않음: %q", a.msg)
	}
}
