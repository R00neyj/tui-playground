package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// 자바스크립트 렌더링 (F-13). 자바스크립트로 화면을 그리는 페이지는 서버가 주는 HTML 에 글이 없으므로,
// PC 에 설치된 Chrome/Edge/Brave 를 화면 없이(headless) 띄워 페이지를 그리게 한 뒤 완성된 DOM 에서 글을 뽑음.
// 새 의존 라이브러리 없이 외부 프로그램만 부르고, 브라우저가 없으면 이 기능만 꺼짐.
// 임시 프로필 폴더를 쓰고 끝나면 지우므로 쿠키·기록이 남지 않음. 사이트의 스크립트가 실제로 실행된다는 점에 주의.

const (
	brRenderTimeout = 30 * time.Second
	brRenderMaxOut  = 8 << 20 // 그려진 DOM 은 8MB 까지만 읽음
	brThinRunes     = 200     // 본문이 이보다 짧고
	brThinScripts   = 3       // 스크립트가 이만큼 이상이면 자바스크립트 페이지로 보고 자동으로 다시 그림
)

var errBrNoEngine = errors.New("자바스크립트를 실행할 브라우저(Chrome, Edge, Brave)를 찾지 못함")

// 테스트에서 바꿔 끼울 수 있게 변수로 둠 (테스트가 실제 브라우저를 띄우지 않도록)
var (
	brFindEngine  = brFindEngineDefault
	brRunEngine   = brRunEngineDefault
	browserRender = brRenderPage
)

// brEngineName 은 화면에 보일 브라우저 이름
func brEngineName(exe string) string {
	b := strings.ToLower(filepath.Base(exe))
	switch {
	case strings.Contains(b, "msedge"), strings.Contains(b, "edge"):
		return "Edge"
	case strings.Contains(b, "brave"):
		return "Brave"
	case strings.Contains(b, "chromium"):
		return "Chromium"
	case strings.Contains(b, "chrome"):
		return "Chrome"
	}
	return "브라우저"
}

func brFileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// brFindEngineDefault 는 설치된 Chromium 계열 브라우저 실행 파일 경로를 찾음 (없으면 "").
// 환경 변수 TUI_BROWSER_EXE 로 직접 지정할 수 있음.
func brFindEngineDefault() string {
	if p := os.Getenv("TUI_BROWSER_EXE"); p != "" && brFileExists(p) {
		return p
	}
	var cands []string
	switch runtime.GOOS {
	case "windows":
		for _, base := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("LocalAppData")} {
			if base == "" {
				continue
			}
			cands = append(cands,
				filepath.Join(base, `Google\Chrome\Application\chrome.exe`),
				filepath.Join(base, `Microsoft\Edge\Application\msedge.exe`),
				filepath.Join(base, `BraveSoftware\Brave-Browser\Application\brave.exe`))
		}
	case "darwin":
		cands = []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		}
	default:
		for _, n := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "microsoft-edge", "brave-browser"} {
			if p, err := exec.LookPath(n); err == nil {
				return p
			}
		}
	}
	for _, p := range cands {
		if brFileExists(p) {
			return p
		}
	}
	return ""
}

// brCapBuf 는 정해진 크기까지만 담는 버퍼 (넘치는 출력은 버림)
type brCapBuf struct {
	b   bytes.Buffer
	max int
}

func (c *brCapBuf) Write(p []byte) (int, error) {
	if room := c.max - c.b.Len(); room > 0 {
		if len(p) > room {
			c.b.Write(p[:room])
		} else {
			c.b.Write(p)
		}
	}
	return len(p), nil
}

// brRunEngineDefault 는 브라우저를 headless 로 실행해 그려진 DOM(HTML)을 돌려줌. ctx 가 끝나면 자식 프로세스까지 모두 끝냄.
func brRunEngineDefault(ctx context.Context, exe, rawURL string) ([]byte, error) {
	dir, err := os.MkdirTemp("", "tui-render-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	args := []string{
		"--headless=new", "--disable-gpu", "--disable-extensions", "--disable-sync",
		"--no-first-run", "--no-default-browser-check", "--mute-audio", "--hide-scrollbars",
		"--window-size=1280,2400",
		"--user-data-dir=" + dir,
		"--virtual-time-budget=10000", // 스크립트가 도는 동안 가상 시간을 앞당겨 로딩을 기다림
		"--dump-dom",
		rawURL, // http(s):// 로 시작하는 주소만 들어옴 (brNormalizeURL) → 옵션으로 해석될 수 없음
	}
	cmd := exec.Command(exe, args...)
	out := &brCapBuf{max: brRenderMaxOut}
	cmd.Stdout = out
	brPrepCmd(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil && out.b.Len() == 0 {
			return nil, err
		}
		return out.b.Bytes(), nil
	case <-ctx.Done():
		brKillTree(cmd)
		<-done
		return nil, ctx.Err()
	}
}

// brRenderPage 는 자바스크립트를 실행해 그려진 결과에서 글을 뽑음
func brRenderPage(ctx context.Context, raw string) (*brPage, error) {
	norm, err := brNormalizeURL(raw)
	if err != nil {
		return nil, err
	}
	norm = brSiteRewrite(norm)
	exe := brFindEngine()
	if exe == "" {
		return nil, errBrNoEngine
	}
	ctx, cancel := context.WithTimeout(ctx, brRenderTimeout)
	defer cancel()
	out, err := brRunEngine(ctx, exe, norm)
	if err != nil {
		switch {
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			return nil, fmt.Errorf("시간 초과 (%d초 안에 그리지 못함)", int(brRenderTimeout.Seconds()))
		case errors.Is(ctx.Err(), context.Canceled):
			return nil, errBrCanceled
		}
		return nil, fmt.Errorf("브라우저 실행 실패: %s", brClip(brClean(err.Error()), 100))
	}
	if len(bytes.TrimSpace(out)) == 0 {
		return nil, errors.New("내용이 없음")
	}
	u, err := url.Parse(norm)
	if err != nil {
		return nil, errors.New("주소 형식이 올바르지 않음")
	}
	p := brBuildPage(out, "text/html; charset=utf-8", false, u)
	p.Rendered = true
	return p, nil
}

// brTextRunes 는 문서 본문 글자 수
func brTextRunes(p *brPage) int {
	n := 0
	for _, b := range p.Blocks {
		for _, r := range b.runs {
			n += len([]rune(r.t))
		}
	}
	return n
}

// brThin 은 서버가 준 HTML 에 글이 거의 없는데 스크립트는 많은 페이지(= 자바스크립트로 그리는 페이지)인지
func brThin(p *brPage) bool {
	return p != nil && !p.Rendered && p.Scripts >= brThinScripts && brTextRunes(p) < brThinRunes
}

// 자바스크립트 렌더링 방식: 자동(서버가 준 HTML 에 글이 거의 없을 때만), 항상, 끔
const (
	brJSAuto = iota
	brJSOn
	brJSOff
)

var brJSNames = [...]string{"자동", "항상 켬", "끔"}

// brLoadPage 는 설정한 방식으로 페이지를 읽음. 자동이면 먼저 보통 방식으로 읽고, 글이 거의 없는 자바스크립트 페이지일 때만 브라우저로 다시 그림.
func brLoadPage(ctx context.Context, raw string, mode int, fetch, render func(context.Context, string) (*brPage, error)) (*brPage, error) {
	if mode == brJSOn {
		return render(ctx, raw)
	}
	p, err := fetch(ctx, raw)
	if err != nil || mode == brJSOff || !brThin(p) || brFindEngine() == "" {
		return p, err
	}
	p2, err2 := render(ctx, raw)
	if errors.Is(err2, errBrCanceled) {
		return nil, err2
	}
	if err2 == nil && brTextRunes(p2) > brTextRunes(p) {
		return p2, nil
	}
	return p, nil
}
