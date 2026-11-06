package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// 실제 Chrome/Edge 를 띄우는 확인용 테스트. TUI_LIVE_SMOKE=1 일 때만 돌고, 로컬 테스트 서버만 씀.

const smokePage = `<!doctype html><html><head><title>스모크 페이지</title></head><body style="font-family:sans-serif">
<nav><a href="/a">홈</a> <a href="/b">소개</a> <button id="b1">로그인</button></nav>
<h1>제목입니다</h1>
<p>첫 문단입니다. 한글과 English 가 섞여 있고 줄바꿈이 일어날 만큼 충분히 길게 써서 폭을 넘기는지 확인함.</p>
<form action="/search" method="get"><input name="q" placeholder="검색어"> <button type="submit">검색</button></form>
<label><input type="checkbox" id="c1"> 동의함</label>
<select id="s1"><option>사과</option><option>배</option></select>
<div id="out"></div>
<div id="host"></div>
<iframe id="fr" src="/frame" style="width:300px;height:60px;border:0"></iframe>
<script>document.getElementById('b1').onclick=function(){document.getElementById('out').textContent='클릭됨 OK'};var sh=document.getElementById('host').attachShadow({mode:'open'});sh.innerHTML='<p>섀도 안의 글</p><button id="sb">섀도 버튼</button>'</script>
</body></html>`

func TestLiveSmoke(t *testing.T) {
	if os.Getenv("TUI_LIVE_SMOKE") == "" {
		t.Skip("TUI_LIVE_SMOKE 가 없어서 건너뜀")
	}
	exe := brFindEngineDefault()
	if exe == "" {
		t.Skip("Chrome/Edge 없음")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/frame" {
			fmt.Fprint(w, "<html><body style=\"margin:0\">프레임 안의 글 <a href=\"/fa\">프레임 링크</a></body></html>")
			return
		}
		if r.URL.Path == "/search" {
			fmt.Fprintf(w, "<html><title>결과</title><body><h2>검색 결과: %s</h2></body></html>", r.URL.Query().Get("q"))
			return
		}
		fmt.Fprint(w, smokePage)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	ch, err := brOpenChrome(ctx, exe, 80, 30)
	if err != nil {
		t.Fatal(err)
	}
	defer ch.Close()
	if err := ch.Navigate(ctx, srv.URL+"/"); err != nil {
		t.Fatal(err)
	}
	snap, err := ch.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sc := brLayoutSnap(snap, 80)
	dump := func() string {
		var b strings.Builder
		for _, r := range sc.rows {
			line := ""
			for _, it := range r {
				line += strings.Repeat(" ", it.col-len([]rune(line))) + it.text
			}
			b.WriteString(line + "\n")
		}
		return b.String()
	}
	t.Log("\n" + dump())
	brMustContain(t, "화면", dump(), "제목입니다", "[ 검색어", "검색 ]", "동의함", "[ ]", "섀도 안의 글", "[ 섀도 버튼 ]", "프레임 안의 글", "프레임 링크")

	// 버튼 클릭
	var bx, by float64
	for _, e := range snap.Elems {
		if e.Kind == "button" && strings.Contains(e.Label, "로그인") {
			bx, by = e.X+e.W/2, e.Y+e.H/2
		}
	}
	if bx == 0 {
		t.Fatal("로그인 버튼을 못 찾음")
	}
	if err := ch.Click(ctx, bx, by); err != nil {
		t.Fatal(err)
	}
	snap, _ = ch.Snapshot(ctx)
	sc = brLayoutSnap(snap, 80)
	brMustContain(t, "클릭 뒤", dump(), "클릭됨 OK")

	// 입력창에 타자 → Enter 로 폼 제출
	for _, e := range snap.Elems {
		if e.Kind == "input" {
			if err := ch.Click(ctx, e.X+e.W/2, e.Y+e.H/2); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := ch.InsertText(ctx, "고양이"); err != nil {
		t.Fatal(err)
	}
	if err := ch.Key(ctx, "enter"); err != nil {
		t.Fatal(err)
	}
	snap, _ = ch.Snapshot(ctx)
	sc = brLayoutSnap(snap, 80)
	t.Log("\n" + dump())
	brMustContain(t, "제출 뒤", dump(), "검색 결과: 고양이")
}

func TestLiveSmokeAppView(t *testing.T) {
	if os.Getenv("TUI_LIVE_SMOKE") == "" {
		t.Skip("TUI_LIVE_SMOKE 가 없어서 건너뜀")
	}
	exe := brFindEngineDefault()
	if exe == "" {
		t.Skip("Chrome/Edge 없음")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, smokePage) }))
	defer srv.Close()
	oldF := brFindEngine
	brFindEngine = brFindEngineDefault
	defer func() { brFindEngine = oldF }()
	a := newBrowserApp()
	defer a.Close()
	c := testCtx(100, 30)
	a.View(c)
	a.Update(key("o"))
	target := srv.URL
	if u := os.Getenv("TUI_LIVE_URL"); u != "" {
		target = u
	}
	a.input.SetValue(target)
	_, cmd := a.Update(key("enter"))
	liveRunWait(t, a, cmd)
	t.Log("\n" + a.View(c))
	a.Update(key("tab"))
	a.Update(key("tab"))
	t.Log("\n" + a.View(c))
}

func liveRunWait(t *testing.T, a *browserApp, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		return
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case msg := <-done:
		switch m := msg.(type) {
		case tea.BatchMsg:
			for _, c := range m {
				if c != nil {
					liveRunWait(t, a, c)
				}
			}
		case brLiveMsg:
			_, next := a.Update(m)
			liveRunWait(t, a, next)
		}
	case <-time.After(25 * time.Second):
	}
}

func TestLiveSmokeScrollLag(t *testing.T) {
	u := os.Getenv("TUI_LIVE_URL")
	if os.Getenv("TUI_LIVE_SMOKE") == "" || u == "" {
		t.Skip("TUI_LIVE_SMOKE 와 TUI_LIVE_URL 이 있을 때만 돌림")
	}
	exe := brFindEngineDefault()
	if exe == "" {
		t.Skip("Chrome/Edge 없음")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	ch, err := brOpenChrome(ctx, exe, 90, 21)
	if err != nil {
		t.Fatal(err)
	}
	defer ch.Close()
	if err := ch.Navigate(ctx, u); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		if err := ch.Scroll(ctx, 40, 100, 340); err != nil {
			t.Fatal(err)
		}
		t0 := time.Now()
		a, _ := ch.Snapshot(ctx)
		t.Logf("snapshot %v", time.Since(t0))
		time.Sleep(1200 * time.Millisecond)
		b, _ := ch.Snapshot(ctx)
		la, lb := brLayoutSnap(a, 90), brLayoutSnap(b, 90)
		if i == 7 {
			for _, r := range lb.rows {
				line := ""
				for _, it := range r {
					line += strings.Repeat(" ", max(0, it.col-lipglossWidth(line))) + it.text
				}
				t.Log(line)
			}
		}
		t.Logf("step %d: 직후 sy=%.0f texts=%d rows=%d | 1.2초 뒤 sy=%.0f texts=%d rows=%d", i, a.SY, len(a.Texts), len(la.rows), b.SY, len(b.Texts), len(lb.rows))
	}
}

func lipglossWidth(s string) int { return lipgloss.Width(s) }

func TestLiveSmokeAppScroll(t *testing.T) {
	u := os.Getenv("TUI_LIVE_URL")
	if os.Getenv("TUI_LIVE_SMOKE") == "" || u == "" {
		t.Skip("TUI_LIVE_SMOKE 와 TUI_LIVE_URL 이 있을 때만 돌림")
	}
	oldF := brFindEngine
	brFindEngine = brFindEngineDefault
	defer func() { brFindEngine = oldF }()
	a := newBrowserApp()
	defer a.Close()
	c := testCtx(100, 30)
	a.View(c)
	a.Update(key("o"))
	a.input.SetValue(u)
	_, cmd := a.Update(key("enter"))
	liveRunWait(t, a, cmd)
	a.View(c)
	for i := 0; i < 6; i++ {
		t0 := time.Now()
		_, cmd = a.Update(brKey(" "))
		liveRunWait(t, a, cmd)
		a.View(c)
		x, y := a.ls.scr.cellPx(a.ls.prow, a.ls.pcol)
		if i == 4 {
			t.Log("\n" + a.View(c))
		}
		t.Logf("space %d: %v sy=%.0f rows=%d cursor=(%d,%d)→px(%.0f,%.0f) vh=%d", i, time.Since(t0).Round(time.Millisecond), a.ls.snap.SY, len(a.ls.scr.rows), a.ls.prow, a.ls.pcol, x, y, a.vh)
	}
}
