package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

// 빠른 이동·검색·자바스크립트 렌더링 (F-13 확장). 실제 네트워크와 실제 브라우저는 쓰지 않음.

func brResetMarks(t *testing.T) {
	t.Helper()
	os.Remove(brMarksPath())
	t.Cleanup(func() { os.Remove(brMarksPath()) })
}

func TestBrResolveInput(t *testing.T) {
	cases := []struct {
		in     string
		search bool
		want   string
	}{
		{"example.com", false, "example.com"},
		{"https://example.com/a b", true, ""}, // 공백이 있으면 검색어
		{"go.dev/blog", false, "go.dev/blog"},
		{"localhost:8080", false, "localhost:8080"},
		{"localhost", false, "localhost"},
		{"10.0.0.1", false, "10.0.0.1"},
		{"javascript:alert(1)", false, "javascript:alert(1)"}, // 주소로 보내 brNormalizeURL 이 거절하게 함
		{"file:///c:/x", false, "file:///c:/x"},
		{"고양이", true, ""},
		{"go", true, ""},
		{"  고양이 사진  ", true, ""},
	}
	for _, c := range cases {
		got, search := brResolveInput(c.in)
		if search != c.search {
			t.Errorf("%q: 검색 여부 = %v, 기대 %v", c.in, search, c.search)
		}
		if !c.search && got != strings.TrimSpace(c.in) {
			t.Errorf("%q: 주소 = %q", c.in, got)
		}
		if c.search && !strings.HasPrefix(got, brSearchBase) {
			t.Errorf("%q: 검색 주소 = %q", c.in, got)
		}
	}
	if got, _ := brResolveInput("고양이 사진"); got != brSearchBase+"%EA%B3%A0%EC%96%91%EC%9D%B4+%EC%82%AC%EC%A7%84" {
		t.Errorf("검색어가 인코딩되지 않음: %q", got)
	}
	// 검색어에 제어 문자가 있어도 주소로 새어나가지 않음
	if got, _ := brResolveInput("a b\x1b[31m"); strings.ContainsRune(got, 0x1b) {
		t.Errorf("검색 주소에 ESC 가 남음: %q", got)
	}
	// 매우 긴 검색어는 잘라냄
	if got, _ := brResolveInput(strings.Repeat("가 ", 500)); len(got) > brMaxURL {
		t.Errorf("검색 주소가 너무 김: %d", len(got))
	}
}

func TestBrSearchFlow(t *testing.T) {
	brResetMarks(t)
	q := brSearchURL("고양이 사진")
	web := &brFakeWeb{pages: map[string]string{q: brLongDoc(2)}}
	brWithFetch(t, web.fetch)
	a := newBrowserApp()
	brOpen(t, a, "고양이 사진")
	if got := web.fetched(); len(got) != 1 || got[0] != q {
		t.Fatalf("검색 주소로 요청해야 함: %v", got)
	}
	if a.cur == nil || a.mode != brRead {
		t.Fatal("검색 결과가 열려야 함")
	}
}

func TestBrMarksDefaultsAndPersistence(t *testing.T) {
	brResetMarks(t)
	got := brLoadMarks()
	if len(got) != len(brDefaultMarks) || got[0].URL != brDefaultMarks[0].URL {
		t.Fatalf("파일이 없으면 기본 목록이어야 함: %v", got)
	}
	web := &brFakeWeb{pages: map[string]string{
		"https://site.com/a": brLongDoc(2),
	}}
	brWithFetch(t, web.fetch)
	a := newBrowserApp()
	brOpen(t, a, "site.com/a")
	a.Update(key("b"))
	if a.msgTone != ToneOk || len(a.marks) != len(brDefaultMarks)+1 {
		t.Fatalf("b 로 추가돼야 함: msg=%q marks=%d", a.msg, len(a.marks))
	}
	a.Update(key("b"))
	if !strings.Contains(a.msg, "이미") || len(a.marks) != len(brDefaultMarks)+1 {
		t.Fatalf("같은 주소는 한 번만: %q", a.msg)
	}
	b := newBrowserApp() // 새 창에서도 보임
	if len(b.marks) != len(brDefaultMarks)+1 || b.marks[len(b.marks)-1].URL != "https://site.com/a" {
		t.Fatalf("저장되지 않음: %v", b.marks)
	}
	// 아홉 개를 넘기지 않음
	for i := 0; len(a.marks) < brMaxMarks; i++ {
		a.marks = append(a.marks, brMark{Title: "x", URL: "https://x.test/" + string(rune('a'+i))})
	}
	a.cur.URL = "https://site.com/other"
	a.Update(key("b"))
	if a.msgTone != ToneWarn || len(a.marks) != brMaxMarks {
		t.Fatalf("개수 제한: %q %d", a.msg, len(a.marks))
	}
}

func TestBrMarksSelectOpenDelete(t *testing.T) {
	brResetMarks(t)
	open := brDefaultMarks[1].URL
	web := &brFakeWeb{pages: map[string]string{open: brLongDoc(2)}}
	brWithFetch(t, web.fetch)
	a := newBrowserApp()
	if a.msel != -1 {
		t.Fatal("처음에는 아무것도 고르지 않은 상태여야 함")
	}
	a.Update(key("enter"))
	if a.loading || !strings.Contains(a.msg, "입력") {
		t.Fatalf("아무것도 안 고르고 Enter: %q", a.msg)
	}
	a.Update(key("down"))
	a.Update(key("down"))
	if a.msel != 1 {
		t.Fatalf("↓ 두 번이면 두 번째 항목: %d", a.msel)
	}
	a.Update(key("up"))
	a.Update(key("up"))
	if a.msel != len(a.marks)-1 { // 1 → 0 → 마지막
		t.Fatalf("위로 넘어가면 마지막으로 돎: %d", a.msel)
	}
	a.msel = 1
	_, cmd := a.Update(key("enter"))
	brDrive(t, a, cmd)
	if got := web.fetched(); len(got) != 1 || got[0] != open {
		t.Fatalf("고른 항목이 열려야 함: %v", got)
	}

	// 지우기: 입력창이 비어 있을 때만
	b := newBrowserApp()
	n := len(b.marks)
	b.msel = 0
	b.Update(key("ctrl+d"))
	if len(b.marks) != n-1 {
		t.Fatalf("Ctrl+D 로 지워야 함: %d", len(b.marks))
	}
	if c := newBrowserApp(); len(c.marks) != n-1 {
		t.Fatalf("지운 결과가 저장돼야 함: %d", len(c.marks))
	}
	// 글자를 치기 시작하면 선택이 풀림
	b.msel = 0
	b.Update(key("x"))
	if b.msel != -1 {
		t.Fatalf("입력을 시작하면 선택이 풀려야 함: %d", b.msel)
	}
	// 입력이 있을 때 Ctrl+D 는 목록을 건드리지 않음
	before := len(b.marks)
	b.msel = 0
	b.Update(key("ctrl+d"))
	if len(b.marks) != before {
		t.Fatal("입력이 있을 때는 목록을 지우면 안 됨")
	}
}

func TestBrMarksSanitize(t *testing.T) {
	brResetMarks(t)
	data := `[{"title":"좋은\u001b[31m곳","url":"https://ok.test/a"},{"title":"나쁨","url":"javascript:alert(1)"},{"title":"","url":"https://no-title.test"},{"title":"스킴 없음","url":"nodots"}]`
	if err := os.WriteFile(brMarksPath(), []byte(data), 0o600); err != nil {
		os.MkdirAll(strings.TrimSuffix(brMarksPath(), "bookmarks.json"), 0o700)
		os.WriteFile(brMarksPath(), []byte(data), 0o600)
	}
	got := brLoadMarks()
	for _, m := range got {
		if strings.ContainsRune(m.Title, 0x1b) || strings.HasPrefix(m.URL, "javascript") {
			t.Fatalf("걸러지지 않음: %+v", m)
		}
	}
	if len(got) < 2 || got[1].Title != "no-title.test" {
		t.Fatalf("제목이 없으면 호스트를 씀: %+v", got)
	}
	// 깨진 파일은 기본 목록
	os.WriteFile(brMarksPath(), []byte("not json"), 0o600)
	if got := brLoadMarks(); len(got) != len(brDefaultMarks) {
		t.Fatalf("깨진 파일은 기본 목록: %v", got)
	}
	// 비어 있는 목록은 그대로 비어 있음
	os.WriteFile(brMarksPath(), []byte("[]"), 0o600)
	if got := brLoadMarks(); len(got) != 0 {
		t.Fatalf("빈 목록은 빈 채로: %v", got)
	}
}

// ---- 자바스크립트 렌더링 ----

func brThinPage() *brPage {
	return &brPage{URL: "https://spa.test/", Title: "SPA", Scripts: 9, Blocks: []brBlock{{kind: bkPara, runs: []brRun{{t: "로딩 중"}}}}}
}

func brRichPage() *brPage {
	long := strings.Repeat("그려진 본문입니다. ", 40)
	return &brPage{URL: "https://spa.test/", Title: "SPA", Rendered: true, Blocks: []brBlock{{kind: bkPara, runs: []brRun{{t: long}}}}}
}

func TestBrThinDefinition(t *testing.T) {
	if !brThin(brThinPage()) {
		t.Error("글이 거의 없고 스크립트가 많으면 얇은 페이지")
	}
	p := brThinPage()
	p.Scripts = 0
	if brThin(p) {
		t.Error("스크립트가 없는 짧은 페이지(example.com 같은)는 다시 그리지 않음")
	}
	if brThin(brRichPage()) || brThin(nil) {
		t.Error("이미 그려진 페이지나 nil 은 얇지 않음")
	}
	long := brThinPage()
	long.Blocks = brRichPage().Blocks
	if brThin(long) {
		t.Error("글이 충분하면 얇지 않음")
	}
}

func TestBrLoadPageModes(t *testing.T) {
	var fetches, renders int
	fetch := func(ctx context.Context, raw string) (*brPage, error) { fetches++; return brThinPage(), nil }
	render := func(ctx context.Context, raw string) (*brPage, error) { renders++; return brRichPage(), nil }
	reset := func() { fetches, renders = 0, 0 }
	old := brFindEngine
	t.Cleanup(func() { brFindEngine = old })

	// 브라우저가 있을 때
	brFindEngine = func() string { return `C:\x\chrome.exe` }
	p, err := brLoadPage(context.Background(), "https://spa.test/", brJSAuto, fetch, render)
	if err != nil || !p.Rendered || fetches != 1 || renders != 1 {
		t.Fatalf("자동: 얇은 페이지는 다시 그려야 함 (rendered=%v f=%d r=%d err=%v)", p != nil && p.Rendered, fetches, renders, err)
	}
	reset()
	p, _ = brLoadPage(context.Background(), "https://spa.test/", brJSOff, fetch, render)
	if p.Rendered || renders != 0 {
		t.Fatal("끔: 다시 그리면 안 됨")
	}
	reset()
	p, _ = brLoadPage(context.Background(), "https://spa.test/", brJSOn, fetch, render)
	if !p.Rendered || fetches != 0 || renders != 1 {
		t.Fatalf("항상: 처음부터 브라우저로 그려야 함 (f=%d r=%d)", fetches, renders)
	}
	// 글이 충분한 페이지는 자동에서 다시 그리지 않음
	reset()
	rich := func(ctx context.Context, raw string) (*brPage, error) {
		fetches++
		p := brRichPage()
		p.Rendered = false
		return p, nil
	}
	p, _ = brLoadPage(context.Background(), "https://blog.test/", brJSAuto, rich, render)
	if p.Rendered || renders != 0 {
		t.Fatal("자동: 글이 충분하면 다시 그리지 않음")
	}
	// 그린 결과가 더 짧으면 원래 결과를 씀
	reset()
	worse := func(ctx context.Context, raw string) (*brPage, error) {
		renders++
		return &brPage{URL: "https://spa.test/", Rendered: true}, nil
	}
	p, _ = brLoadPage(context.Background(), "https://spa.test/", brJSAuto, fetch, worse)
	if p.Rendered {
		t.Fatal("그린 결과가 더 빈약하면 원래 결과를 써야 함")
	}
	// 그리기가 실패하면 원래 결과를 보여줌 (오류로 막지 않음)
	reset()
	failing := func(ctx context.Context, raw string) (*brPage, error) { renders++; return nil, errors.New("실패") }
	p, err = brLoadPage(context.Background(), "https://spa.test/", brJSAuto, fetch, failing)
	if err != nil || p == nil || p.Rendered {
		t.Fatalf("자동: 그리기 실패는 원래 결과로 대체 (err=%v)", err)
	}
	// 취소는 오류로 돌려줌
	cancelled := func(ctx context.Context, raw string) (*brPage, error) { return nil, errBrCanceled }
	if _, err = brLoadPage(context.Background(), "https://spa.test/", brJSAuto, fetch, cancelled); !errors.Is(err, errBrCanceled) {
		t.Fatalf("취소가 전달돼야 함: %v", err)
	}
	// 브라우저가 없으면 자동에서는 그리기를 시도하지 않음
	brFindEngine = func() string { return "" }
	reset()
	p, _ = brLoadPage(context.Background(), "https://spa.test/", brJSAuto, fetch, render)
	if p.Rendered || renders != 0 {
		t.Fatal("브라우저가 없으면 자동에서 시도하지 않음")
	}
	// 항상 켬인데 브라우저가 없으면 이유를 알려줌
	_, err = brLoadPage(context.Background(), "https://spa.test/", brJSOn, fetch, brRenderPage)
	if !errors.Is(err, errBrNoEngine) {
		t.Fatalf("항상 켬 + 브라우저 없음: %v", err)
	}
}

func TestBrRenderPageWithFakeEngine(t *testing.T) {
	oldF, oldR := brFindEngine, brRunEngine
	t.Cleanup(func() { brFindEngine, brRunEngine = oldF, oldR })
	brFindEngine = func() string { return `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe` }
	var gotExe, gotURL string
	brRunEngine = func(ctx context.Context, exe, raw string) ([]byte, error) {
		gotExe, gotURL = exe, raw
		return []byte("<html><head><title>그려진 페이지</title></head><body><main><p>" + strings.Repeat("본문 문장입니다. ", 30) + "</p></main></body></html>"), nil
	}
	p, err := brRenderPage(context.Background(), "spa.test/x")
	if err != nil {
		t.Fatal(err)
	}
	if !p.Rendered || p.Title != "그려진 페이지" || p.URL != "https://spa.test/x" {
		t.Fatalf("그려진 결과: %+v", p)
	}
	if !strings.HasPrefix(gotURL, "https://") || !strings.HasSuffix(gotExe, "msedge.exe") {
		t.Fatalf("엔진 호출: %q %q", gotExe, gotURL)
	}
	if brEngineName(gotExe) != "Edge" || brEngineName("/usr/bin/chromium") != "Chromium" || brEngineName(`C:\a\chrome.exe`) != "Chrome" || brEngineName("brave.exe") != "Brave" {
		t.Error("브라우저 이름 표시")
	}
	// 네이버 블로그 주소는 본문 주소로 바꿔서 그림
	brRenderPage(context.Background(), "https://blog.naver.com/abc/123")
	if !strings.Contains(gotURL, "PostView.naver") {
		t.Errorf("사이트별 주소 변환이 적용돼야 함: %q", gotURL)
	}
	// 옵션처럼 보이는 입력은 엔진에 도달하지 못함
	gotURL = ""
	if _, err := brRenderPage(context.Background(), "--remote-debugging-port=1 x"); err == nil || gotURL != "" {
		// 공백이 들어 있어 주소 검사에서 거절됨
		if gotURL != "" {
			t.Errorf("옵션 문자열이 엔진에 전달됨: %q", gotURL)
		}
	}
	// 오류 분류
	brRunEngine = func(ctx context.Context, exe, raw string) ([]byte, error) { return nil, errors.New("boom") }
	if _, err := brRenderPage(context.Background(), "spa.test"); err == nil || !strings.Contains(err.Error(), "브라우저 실행 실패") {
		t.Errorf("실행 실패: %v", err)
	}
	brRunEngine = func(ctx context.Context, exe, raw string) ([]byte, error) { return []byte("  \n"), nil }
	if _, err := brRenderPage(context.Background(), "spa.test"); err == nil || !strings.Contains(err.Error(), "내용이 없음") {
		t.Errorf("빈 결과: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	brRunEngine = func(c context.Context, exe, raw string) ([]byte, error) { cancel(); <-c.Done(); return nil, c.Err() }
	if _, err := brRenderPage(ctx, "spa.test"); !errors.Is(err, errBrCanceled) {
		t.Errorf("취소: %v", err)
	}
	brFindEngine = func() string { return "" }
	if _, err := brRenderPage(context.Background(), "spa.test"); !errors.Is(err, errBrNoEngine) {
		t.Errorf("브라우저 없음: %v", err)
	}
}

func TestBrFindEngineEnvOverride(t *testing.T) {
	f := t.TempDir() + string(os.PathSeparator) + "mychrome.exe"
	os.WriteFile(f, []byte("x"), 0o600)
	t.Setenv("TUI_BROWSER_EXE", f)
	if got := brFindEngineDefault(); got != f {
		t.Fatalf("TUI_BROWSER_EXE 를 따라야 함: %q", got)
	}
	t.Setenv("TUI_BROWSER_EXE", f+".missing")
	if got := brFindEngineDefault(); got == f+".missing" {
		t.Fatal("없는 파일은 무시해야 함")
	}
}

func TestBrJSModeKeyAndRenderedNote(t *testing.T) {
	brResetMarks(t)
	oldF, oldR := brFindEngine, browserRender
	t.Cleanup(func() { brFindEngine, browserRender = oldF, oldR })
	brFindEngine = func() string { return `C:\x\msedge.exe` }
	browserRender = func(ctx context.Context, raw string) (*brPage, error) { return brRichPage(), nil }
	spaDoc := "<html><head><title>SPA</title></head><body><script></script><script></script><script></script><div id=app>로딩</div></body></html>"
	web := &brFakeWeb{pages: map[string]string{"https://spa.test": spaDoc, "https://spa.test/": spaDoc}}
	brWithFetch(t, web.fetch)
	a := newBrowserApp()
	brOpen(t, a, "spa.test")
	if a.cur == nil || !a.cur.Rendered || !strings.Contains(a.msg, "Edge") {
		t.Fatalf("자동 모드에서 얇은 페이지는 브라우저로 다시 그려야 함: rendered=%v msg=%q", a.cur != nil && a.cur.Rendered, a.msg)
	}
	a.Update(key("m"))
	if a.jsMode != brJSOn || !strings.Contains(a.msg, "항상") {
		t.Fatalf("m: 자동 → 항상 켬: %d %q", a.jsMode, a.msg)
	}
	a.Update(key("m"))
	a.Update(key("m"))
	if a.jsMode != brJSAuto {
		t.Fatalf("m 세 번이면 처음으로: %d", a.jsMode)
	}
	a.Update(key("m"))
	a.Update(key("m")) // 끔
	if a.jsMode != brJSOff {
		t.Fatalf("끔: %d", a.jsMode)
	}
	_, cmd := a.Update(key("r"))
	brDrive(t, a, cmd)
	if a.cur.Rendered {
		t.Fatal("끔이면 다시 그리지 않아야 함")
	}
}

func TestBrInputScreenAlignedWithMarks(t *testing.T) {
	brResetMarks(t)
	oldF := brFindEngine
	t.Cleanup(func() { brFindEngine = oldF })
	for _, engine := range []string{"", `C:\x\chrome.exe`} {
		brFindEngine = func() string { return engine }
		for _, n := range []int{0, 1, 4, brMaxMarks} {
			for _, sel := range []int{-1, 0, n - 1} {
				for _, sz := range testSizes {
					a := newBrowserApp()
					a.marks = nil
					for i := 0; i < n; i++ {
						a.marks = append(a.marks, brMark{Title: "한글 제목이 아주 길어서 잘려야 하는 북마크 " + string(rune('A'+i)), URL: "https://example" + string(rune('a'+i)) + ".com/path"})
					}
					a.msel = sel
					c := testCtx(sz[0], sz[1])
					v := a.View(c)
					assertFrame(t, "browser-input", c, v)
					if lines := strings.Count(v, "\n") + 1; lines > sz[1] {
						t.Fatalf("n=%d sel=%d %dx%d: %d줄로 창 높이를 넘음", n, sel, sz[0], sz[1], lines)
					}
					if sel >= 0 && sel < n && !strings.Contains(plain(v), "▸") {
						t.Fatalf("n=%d sel=%d %dx%d: 선택 표시가 보이지 않음\n%s", n, sel, sz[0], sz[1], plain(v))
					}
				}
			}
		}
	}
}
