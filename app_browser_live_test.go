package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// 라이브 모드 테스트. 실제 Chrome 을 띄우지 않고 가짜 세션(fakeLive)을 끼워 씀.

type fakeLive struct {
	mu     sync.Mutex
	snap   brSnap
	calls  []string
	closed bool
	navErr error
	block  bool
}

func (f *fakeLive) log(s string) {
	f.mu.Lock()
	f.calls = append(f.calls, s)
	f.mu.Unlock()
}

func (f *fakeLive) got() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeLive) Navigate(ctx context.Context, u string) error {
	f.log("nav " + u)
	if f.block {
		<-ctx.Done()
		return ctx.Err()
	}
	if f.navErr != nil {
		return f.navErr
	}
	f.mu.Lock()
	f.snap.URL = u
	f.mu.Unlock()
	return nil
}

func (f *fakeLive) Snapshot(ctx context.Context) (*brSnap, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := f.snap
	s.Texts = append([]brSnapText(nil), f.snap.Texts...)
	s.Elems = append([]brSnapElem(nil), f.snap.Elems...)
	return &s, nil
}

func (f *fakeLive) Click(ctx context.Context, x, y float64) error {
	f.log(fmt.Sprintf("click %.0f,%.0f", x, y))
	return nil
}

func (f *fakeLive) InsertText(ctx context.Context, s string) error {
	f.log("text " + s)
	f.mu.Lock()
	for i := range f.snap.Elems {
		if f.snap.Elems[i].Kind == "input" {
			f.snap.Elems[i].Value += s
		}
	}
	f.mu.Unlock()
	return nil
}

func (f *fakeLive) Key(ctx context.Context, n string) error { f.log("key " + n); return nil }
func (f *fakeLive) Scroll(ctx context.Context, x, y, dy float64) error {
	f.log(fmt.Sprintf("scroll %.0f", dy))
	return nil
}
func (f *fakeLive) ScrollTo(ctx context.Context, top bool) error {
	f.log(fmt.Sprint("scrollto ", top))
	return nil
}
func (f *fakeLive) SelectNext(ctx context.Context, i int) error {
	f.log(fmt.Sprint("select ", i))
	return nil
}
func (f *fakeLive) History(ctx context.Context, back bool) error {
	f.log(fmt.Sprint("history ", back))
	return nil
}
func (f *fakeLive) Reload(ctx context.Context) error { f.log("reload"); return nil }
func (f *fakeLive) Resize(ctx context.Context, cols, rows int) error {
	f.log(fmt.Sprint("resize ", cols, rows))
	return nil
}
func (f *fakeLive) Close() { f.log("close"); f.mu.Lock(); f.closed = true; f.mu.Unlock() }

func fakeSnap() brSnap {
	return brSnap{
		URL: "https://live.test/", Title: "라이브 테스트", DH: 2000, VH: 540,
		Texts: []brSnapText{
			{T: "홈", X: 8, Y: 10, W: 16, H: 18, F: 0},
			{T: "소개", X: 40, Y: 10, W: 32, H: 18, F: 1},
			{T: "큰 제목", X: 8, Y: 60, W: 100, H: 24, F: -1, B: 1, H1: 1},
			{T: "본문 글이 여기에 있음", X: 8, Y: 110, W: 200, H: 18, F: -1},
		},
		Elems: []brSnapElem{
			{Kind: "link", Label: "홈", Href: "https://live.test/home", X: 8, Y: 10, W: 16, H: 18},
			{Kind: "link", Label: "소개", Href: "https://other.test/about", X: 40, Y: 10, W: 32, H: 18},
			{Kind: "input", Type: "text", Ph: "검색어", X: 8, Y: 160, W: 240, H: 28, Widget: 1},
			{Kind: "button", Label: "검색", X: 260, Y: 160, W: 60, H: 28, Widget: 1},
		},
	}
}

func withFakeLive(t *testing.T) *fakeLive {
	t.Helper()
	f := &fakeLive{snap: fakeSnap()}
	oldF, oldO := brFindEngine, brOpenLive
	t.Cleanup(func() { brFindEngine, brOpenLive = oldF, oldO })
	brFindEngine = func() string { return `C:\x\chrome.exe` }
	brOpenLive = func(ctx context.Context, exe string, cols, rows int) (brLive, error) {
		f.log(fmt.Sprint("open ", cols, rows))
		return f, nil
	}
	return f
}

// liveRun 은 명령을 실행해 돌아온 라이브 메시지를 앱에 먹이고, 이어지는 명령도 끝까지 처리함 (틱은 무시)
func liveRun(t *testing.T, a *browserApp, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		return
	}
	done := make(chan tea.Msg, 8)
	go func() { done <- cmd() }()
	select {
	case msg := <-done:
		switch m := msg.(type) {
		case tea.BatchMsg:
			for _, c := range m {
				if c != nil {
					liveRun(t, a, c)
				}
			}
		case brLiveMsg:
			_, next := a.Update(m)
			liveRun(t, a, next)
		}
	case <-time.After(250 * time.Millisecond): // 틱처럼 기다리는 명령은 건너뜀
	}
}

func liveOpen(t *testing.T, a *browserApp, addr string) {
	t.Helper()
	a.Update(key("o"))
	a.input.SetValue(addr)
	_, cmd := a.Update(key("enter"))
	liveRun(t, a, cmd)
}

func liveView(a *browserApp) string { return a.View(testCtx(100, 30)) }

func TestLiveOpenRendersPage(t *testing.T) {
	brResetMarks(t)
	f := withFakeLive(t)
	a := newBrowserApp()
	liveOpen(t, a, "live.test")
	if !a.ls.on || a.mode != brRead {
		t.Fatalf("라이브 읽기 상태여야 함: on=%v mode=%v msg=%q", a.ls.on, a.mode, a.msg)
	}
	v := liveView(a)
	brMustContain(t, "화면", v, "라이브 테스트", "큰 제목", "본문 글이 여기에 있음", "[ 검색어", "[ 검색 ]")
	if got := f.got(); len(got) < 2 || !strings.HasPrefix(got[0], "open ") || got[1] != "nav https://live.test" {
		t.Fatalf("열고 이동해야 함: %v", got)
	}
	// 첫 번째 누를 수 있는 곳에 커서가 놓임
	if a.ls.prow != 0 {
		t.Fatalf("커서는 첫 요소 줄: %d,%d", a.ls.prow, a.ls.pcol)
	}
}

func TestLiveCursorMovesAndClicks(t *testing.T) {
	brResetMarks(t)
	f := withFakeLive(t)
	a := newBrowserApp()
	liveOpen(t, a, "live.test")
	r0, c0 := a.ls.prow, a.ls.pcol
	a.Update(key("right"))
	if a.ls.pcol <= c0 || a.ls.prow != r0 {
		t.Fatalf("오른쪽으로 한 칸 이상 움직여야 함: %d→%d", c0, a.ls.pcol)
	}
	// 첫 링크(홈) 위로 되돌아와서 Enter → 그 위치를 실제 클릭
	a.ls.prow, a.ls.pcol = a.ls.scr.elemP[0][0], a.ls.scr.elemP[0][1]
	if !strings.Contains(liveView(a), "→ https://live.test/home") {
		t.Fatalf("커서 아래 링크 주소가 상태줄에 보여야 함:\n%s", liveView(a))
	}
	_, cmd := a.Update(key("enter"))
	liveRun(t, a, cmd)
	var clicked string
	for _, c := range f.got() {
		if strings.HasPrefix(c, "click ") {
			clicked = c
		}
	}
	if clicked == "" {
		t.Fatalf("Enter 는 클릭을 보내야 함: %v", f.got())
	}
	var x, y float64
	fmt.Sscanf(clicked, "click %f,%f", &x, &y)
	if x < 8 || x > 24 || y < 10 || y > 28 {
		t.Fatalf("클릭 좌표가 링크 안이어야 함: %s", clicked)
	}
}

func TestLiveCrossSiteHoverIsWarn(t *testing.T) {
	brResetMarks(t)
	withFakeLive(t)
	a := newBrowserApp()
	liveOpen(t, a, "live.test")
	a.ls.prow, a.ls.pcol = a.ls.scr.elemP[1][0], a.ls.scr.elemP[1][1]
	msg, tone, ok := a.liveStatus()
	if !ok || !strings.Contains(msg, "other.test/about") || tone != ToneWarn {
		t.Fatalf("다른 사이트 링크는 경고색으로 주소를 보여야 함: %q %v", msg, tone)
	}
}

func TestLiveEditModeTypesAndEscExitsInputOnly(t *testing.T) {
	brResetMarks(t)
	f := withFakeLive(t)
	a := newBrowserApp()
	liveOpen(t, a, "live.test")
	a.ls.prow, a.ls.pcol = a.ls.scr.elemP[2][0], a.ls.scr.elemP[2][1]
	_, cmd := a.Update(key("enter"))
	liveRun(t, a, cmd)
	if !a.ls.edit {
		t.Fatal("입력창에서 Enter 는 입력 모드로 들어가야 함")
	}
	for _, r := range "고양이" {
		_, cmd = a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		liveRun(t, a, cmd)
	}
	if !strings.Contains(liveView(a), "고양이") {
		t.Fatalf("입력한 글자가 화면에 보여야 함:\n%s", liveView(a))
	}
	_, cmd = a.Update(key("enter"))
	liveRun(t, a, cmd)
	joined := strings.Join(f.got(), "|")
	if !strings.Contains(joined, "key enter") {
		t.Fatalf("Enter 는 폼 제출 키로 전달: %v", f.got())
	}
	_, cmd = a.Update(key("esc"))
	if a.ls.edit || cmd != nil {
		t.Fatalf("입력 중 Esc 는 입력만 끝내야 함 (홈으로 가면 안 됨): edit=%v", a.ls.edit)
	}
	_, cmd = a.Update(key("esc"))
	if cmd == nil {
		t.Fatal("입력 중이 아니면 Esc 는 홈으로")
	}
	if _, ok := cmd().(goHomeMsg); !ok {
		t.Fatal("goHomeMsg 여야 함")
	}
	if !f.closed {
		t.Fatal("홈으로 갈 때 브라우저 세션을 닫아야 함")
	}
}

func TestLiveTypingIsOrderedAndMerged(t *testing.T) {
	brResetMarks(t)
	f := withFakeLive(t)
	a := newBrowserApp()
	liveOpen(t, a, "live.test")
	a.ls.live = f
	a.ls.edit = true
	var first tea.Cmd
	for i, r := range "abc" {
		_, c := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		if i == 0 {
			first = c
		} else if c != nil {
			t.Fatal("처리 중에 친 글자는 대기열에 쌓여야 함 (동시에 보내면 순서가 섞임)")
		}
	}
	liveRun(t, a, first)
	var texts []string
	for _, c := range f.got() {
		if strings.HasPrefix(c, "text ") {
			texts = append(texts, c)
		}
	}
	if strings.Join(texts, "") != "text atext bc" && strings.Join(texts, "") != "text abc" {
		t.Fatalf("글자 순서가 유지돼야 함: %v", texts)
	}
}

func TestLiveFallsBackToReaderWhenBrowserWontOpen(t *testing.T) {
	brResetMarks(t)
	oldF, oldO := brFindEngine, brOpenLive
	t.Cleanup(func() { brFindEngine, brOpenLive = oldF, oldO })
	brFindEngine = func() string { return `C:\x\chrome.exe` }
	brOpenLive = func(ctx context.Context, exe string, cols, rows int) (brLive, error) {
		return nil, errors.New("실행 실패")
	}
	web := &brFakeWeb{pages: map[string]string{"https://plain.test": "<html><title>일반</title><body><p>읽을 글</p></body></html>", "https://plain.test/": "<html><title>일반</title><body><p>읽을 글</p></body></html>"}}
	brWithFetch(t, web.fetch)
	a := newBrowserApp()
	liveOpen(t, a, "plain.test")
	if a.ls.on || a.cur == nil || a.cur.Title != "일반" {
		t.Fatalf("리더 모드로 읽어야 함: on=%v cur=%+v", a.ls.on, a.cur)
	}
	if !strings.Contains(a.msg, "리더 모드") {
		t.Fatalf("라이브 실패 안내가 있어야 함: %q", a.msg)
	}
}

func TestLiveToggleReaderClosesSession(t *testing.T) {
	brResetMarks(t)
	f := withFakeLive(t)
	web := &brFakeWeb{pages: map[string]string{"https://live.test": "<html><title>글</title><body><p>리더 본문</p></body></html>", "https://live.test/": "<html><title>글</title><body><p>리더 본문</p></body></html>"}}
	brWithFetch(t, web.fetch)
	a := newBrowserApp()
	liveOpen(t, a, "live.test")
	_, cmd := a.Update(key("v"))
	brDrive(t, a, cmd)
	if a.ls.on || !f.closed || !strings.Contains(brText(a.cur, 60), "리더 본문") {
		t.Fatalf("v: 라이브를 닫고 리더 모드로: on=%v closed=%v", a.ls.on, f.closed)
	}
}

func TestLiveViewsAligned(t *testing.T) {
	brResetMarks(t)
	withFakeLive(t)
	for _, sz := range testSizes {
		a := newBrowserApp()
		liveOpen(t, a, "live.test")
		c := testCtx(sz[0], sz[1])
		assertFrame(t, fmt.Sprintf("live %dx%d", sz[0], sz[1]), c, a.View(c))
		a.Update(key("right"))
		a.Update(key("down"))
		assertFrame(t, fmt.Sprintf("live moved %dx%d", sz[0], sz[1]), c, a.View(c))
		a.ls.edit = true
		assertFrame(t, fmt.Sprintf("live edit %dx%d", sz[0], sz[1]), c, a.View(c))
	}
}

func TestLiveResizeFollowsWindow(t *testing.T) {
	brResetMarks(t)
	f := withFakeLive(t)
	a := newBrowserApp()
	a.View(testCtx(100, 30))
	a.View(testCtx(100, 30))
	liveOpen(t, a, "live.test")
	a.View(testCtx(100, 30))
	if cmd := a.resizeIfNeeded(); cmd != nil {
		t.Fatal("크기가 같으면 아무것도 보내지 않음")
	}
	a.View(testCtx(70, 30))
	cmd := a.resizeIfNeeded()
	if cmd == nil {
		t.Fatal("폭이 바뀌면 세션 화면 크기를 맞춰야 함")
	}
	liveRun(t, a, cmd)
	found := false
	for _, c := range f.got() {
		found = found || strings.HasPrefix(c, "resize 60 ")
	}
	if !found {
		t.Fatalf("resize 호출이 없음: %v", f.got())
	}
}

func TestParseSnapSanitizes(t *testing.T) {
	raw := `{"u":"https://a.test/","t":"제목\u001b[31m붉게","sy":1e300,"dh":100,"vh":50,` +
		`"texts":[{"t":"안녕\u001b]0;해킹\u0007","x":1e12,"y":2,"w":3,"h":0,"f":99,"b":0,"hd":0,"k":0}],` +
		`"elems":[{"k":"link","l":"x\u202e","hf":"https://a.test/\u001b","x":0,"y":0,"w":1,"h":1}]}`
	s, err := brParseSnap(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, str := range []string{s.Title, s.Texts[0].T, s.Elems[0].Label, s.Elems[0].Href} {
		if strings.ContainsAny(str, "\x1b\x07\u202e") {
			t.Fatalf("제어 문자가 남음: %q", str)
		}
	}
	if s.Texts[0].X > brCoordLimit || s.Texts[0].F != -1 || s.Texts[0].H < 1 {
		t.Fatalf("좌표·참조가 정리돼야 함: %+v", s.Texts[0])
	}
	if _, err := brParseSnap("not json"); err == nil {
		t.Fatal("깨진 JSON 은 오류")
	}
}

func TestLayoutKeepsOverflowingTextAndSeparatesWidgets(t *testing.T) {
	long := strings.Repeat("가나다 ", 40)
	s := &brSnap{
		Texts: []brSnapText{{T: strings.TrimSpace(long), X: 8, Y: 0, W: 600, H: 18, F: -1}},
		Elems: []brSnapElem{{Kind: "input", Type: "text", Ph: "검색", X: 8, Y: 80, W: 400, H: 28, Widget: 1}},
	}
	sc := brLayoutSnap(s, 40)
	var all []string
	for _, r := range sc.rows {
		for _, it := range r {
			all = append(all, it.text)
			if it.col+it.width() > 40 {
				t.Fatalf("칸 폭을 넘음: %+v", it)
			}
		}
	}
	if got := strings.ReplaceAll(strings.Join(all, " "), "  ", " "); !strings.Contains(got, "가나다") || strings.Count(got, "가나다") < 40 {
		t.Fatalf("긴 글이 잘려 사라지면 안 됨: %d개", strings.Count(got, "가나다"))
	}
	if sc.elemP[0][0] < 0 {
		t.Fatal("입력창 위치가 기록돼야 함")
	}
}

func TestLayoutBlankRowBetweenParagraphs(t *testing.T) {
	s := &brSnap{Texts: []brSnapText{
		{T: "하나", X: 0, Y: 0, W: 30, H: 18, F: -1},
		{T: "둘", X: 0, Y: 18, W: 20, H: 18, F: -1},
		{T: "셋", X: 0, Y: 80, W: 20, H: 18, F: -1},
	}}
	sc := brLayoutSnap(s, 40)
	if len(sc.rows) != 4 || len(sc.rows[2]) != 0 {
		t.Fatalf("붙은 줄은 이어 쓰고 떨어진 문단 사이는 빈 줄 하나: %d줄", len(sc.rows))
	}
}

func TestNextAnchorCyclesInReadingOrder(t *testing.T) {
	sc := brLayoutSnap(func() *brSnap { s := fakeSnap(); return &s }(), 80)
	r, c, ok := sc.nextAnchor(-1, 0, 1)
	if !ok || r != 0 {
		t.Fatalf("첫 요소는 첫 줄: %d,%d", r, c)
	}
	seen := 0
	for i := 0; i < 10 && seen < 4; i++ {
		var ok bool
		r, c, ok = sc.nextAnchor(r, c, 1)
		if !ok {
			t.Fatal("요소가 있어야 함")
		}
		seen++
	}
	pr, pc, _ := sc.nextAnchor(r, c, -1)
	nr, nc, _ := sc.nextAnchor(pr, pc, 1)
	if nr != r || nc != c {
		t.Fatalf("앞뒤 이동이 서로 되돌려야 함: (%d,%d)→(%d,%d)", r, c, nr, nc)
	}
}

func TestLiveCancelDuringNavigateClosesBrowser(t *testing.T) {
	brResetMarks(t)
	f := withFakeLive(t)
	f.block = true
	a := newBrowserApp()
	a.Update(key("o"))
	a.input.SetValue("live.test")
	_, cmd := a.Update(key("enter"))
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatal("불러오기 명령이 묶음이어야 함")
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- batch[0]() }()
	time.Sleep(100 * time.Millisecond)
	a.Update(key("esc"))
	select {
	case msg := <-done:
		if m, ok := msg.(brLiveMsg); !ok || m.Live != nil {
			t.Fatalf("취소되면 열린 브라우저를 넘기지 말고 닫아야 함: %+v", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("취소해도 명령이 끝나지 않음")
	}
	f.mu.Lock()
	closed := f.closed
	f.mu.Unlock()
	if !closed {
		t.Fatal("취소하면 방금 연 브라우저가 남지 않게 닫아야 함")
	}
}

func TestLiveNaverBlogUsesPostViewURL(t *testing.T) {
	brResetMarks(t)
	f := withFakeLive(t)
	a := newBrowserApp()
	liveOpen(t, a, "https://blog.naver.com/someone/223456789012")
	var nav string
	for _, c := range f.got() {
		if strings.HasPrefix(c, "nav ") {
			nav = c
		}
	}
	if !strings.Contains(nav, "PostView.naver") {
		t.Fatalf("네이버 블로그 글은 iframe 없는 PostView 주소로 열어야 함: %q", nav)
	}
}

func TestLiveDenseRowsPageBeforeScrollingThePage(t *testing.T) {
	brResetMarks(t)
	f := withFakeLive(t)
	f.snap.Texts = nil
	for i := 0; i < 40; i++ {
		f.snap.Texts = append(f.snap.Texts, brSnapText{T: fmt.Sprintf("줄%02d", i), X: 8, Y: float64(i * 18), W: 40, H: 18, F: -1})
	}
	f.snap.Elems = nil
	f.snap.VH = 720
	a := newBrowserApp()
	c := testCtx(100, 30)
	a.View(c)
	liveOpen(t, a, "live.test")
	v := a.View(c)
	if !strings.Contains(v, "줄00") || strings.Contains(v, "줄35") {
		t.Fatalf("처음엔 앞쪽 줄만 보여야 함:\n%s", v)
	}
	scrolls := func() int {
		n := 0
		for _, c := range f.got() {
			if strings.HasPrefix(c, "scroll ") {
				n++
			}
		}
		return n
	}
	_, cmd := a.Update(key(" "))
	liveRun(t, a, cmd)
	if v = a.View(c); !strings.Contains(v, "줄35") || scrolls() != 0 {
		t.Fatalf("Space 는 안 보이던 아랫줄을 먼저 보여줘야 함 (페이지 스크롤 %d회):\n%s", scrolls(), v)
	}
	_, cmd = a.Update(key(" "))
	liveRun(t, a, cmd)
	if !strings.Contains(a.View(c), "줄39") || scrolls() != 0 {
		t.Fatalf("마지막 줄까지 보여야 함 (페이지 스크롤 %d회)", scrolls())
	}
	_, cmd = a.Update(key(" "))
	liveRun(t, a, cmd)
	if scrolls() != 1 {
		t.Fatalf("끝에 닿은 뒤에야 페이지를 스크롤해야 함: %d회", scrolls())
	}
}

func TestLiveCursorDownPastEdgeMovesRowWindowFirst(t *testing.T) {
	brResetMarks(t)
	f := withFakeLive(t)
	f.snap.Texts = nil
	for i := 0; i < 30; i++ {
		f.snap.Texts = append(f.snap.Texts, brSnapText{T: fmt.Sprintf("줄%02d", i), X: 8, Y: float64(i * 18), W: 40, H: 18, F: -1})
	}
	f.snap.Elems = nil
	a := newBrowserApp()
	c := testCtx(100, 30)
	a.View(c)
	liveOpen(t, a, "live.test")
	a.ls.prow = a.vh - 1
	a.Update(key("down"))
	if a.ls.roff != 1 || a.ls.prow != a.vh {
		t.Fatalf("가장자리에서 아래로 가면 줄 창이 한 줄 내려가야 함: roff=%d prow=%d", a.ls.roff, a.ls.prow)
	}
	if !strings.Contains(a.View(c), "줄20") {
		t.Fatal("내려간 줄이 보여야 함")
	}
}

func TestWordWrapNeverHangsOnNarrowWidth(t *testing.T) {
	done := make(chan []string, 1)
	go func() { done <- brWordWrap("한글 日本語 abc 가나다라마바사", 1) }()
	select {
	case got := <-done:
		for _, l := range got {
			if l == "" {
				t.Fatalf("빈 줄이 섞이면 안 됨: %q", got)
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("칸 폭 1 에서 brWordWrap 이 끝나지 않음")
	}
	if got := brWordWrap("abc def", 0); got != nil {
		t.Fatalf("폭 0 이면 비어야 함: %q", got)
	}
}

func resizeCalls(f *fakeLive) []string {
	var out []string
	for _, c := range f.got() {
		if strings.HasPrefix(c, "resize ") {
			out = append(out, c)
		}
	}
	return out
}

func TestLiveRefitGrowsViewportWhenRowsAreSparse(t *testing.T) {
	brResetMarks(t)
	f := withFakeLive(t)
	a := newBrowserApp()
	a.View(testCtx(100, 30))
	liveOpen(t, a, "live.test")
	rs := resizeCalls(f)
	if len(rs) == 0 {
		t.Fatalf("줄이 모자라고 아래에 내용이 더 있으면 화면 높이를 늘려야 함: %v", f.got())
	}
	var cols, rows int
	fmt.Sscanf(rs[0], "resize %d %d", &cols, &rows)
	if rows <= a.vh {
		t.Fatalf("늘린 높이가 화면 줄(%d)보다 커야 함: %d", a.vh, rows)
	}
}

func TestLiveRefitDoesNotGrowAtPageEnd(t *testing.T) {
	brResetMarks(t)
	f := withFakeLive(t)
	f.snap.DH = f.snap.VH
	a := newBrowserApp()
	a.View(testCtx(100, 30))
	liveOpen(t, a, "live.test")
	if rs := resizeCalls(f); len(rs) != 0 {
		t.Fatalf("페이지 끝이면 화면 높이를 늘리지 않아야 함: %v", rs)
	}
}
