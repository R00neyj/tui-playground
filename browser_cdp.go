package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/websocket"
)

// 라이브 세션 (F-13): Chrome/Edge 를 화면 없이 띄우고 DevTools 프로토콜로 조종함. 임시 프로필을 쓰고 끝나면 지움.

const (
	brCellW        = 7  // 터미널 한 칸에 해당하는 페이지 픽셀 폭 (14px 글꼴이 칸 폭을 넘지 않게 함)
	brCellH        = 18 // 터미널 한 줄에 해당하는 페이지 픽셀 높이
	brOpenTimeout  = 20 * time.Second
	brSettleMax    = 12 * time.Second
	brActionBudget = 60 * time.Second
)

var errBrLiveGone = errors.New("브라우저가 종료됨")

// brLive 는 앱이 쓰는 라이브 세션 창구. 테스트에서는 가짜로 바꿔 끼움.
type brLive interface {
	Navigate(ctx context.Context, rawURL string) error
	Snapshot(ctx context.Context) (*brSnap, error)
	Click(ctx context.Context, x, y float64) error
	InsertText(ctx context.Context, s string) error
	Key(ctx context.Context, name string) error
	Scroll(ctx context.Context, x, y, dy float64) error
	ScrollTo(ctx context.Context, top bool) error
	SelectNext(ctx context.Context, idx int) error
	History(ctx context.Context, back bool) error
	Reload(ctx context.Context) error
	Resize(ctx context.Context, cols, rows int) error
	Close()
}

var brOpenLive = func(ctx context.Context, exe string, cols, rows int) (brLive, error) {
	c, err := brOpenChrome(ctx, exe, cols, rows)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// ---- 프로토콜 연결 ----

type cdpReply struct {
	result json.RawMessage
	err    error
}

type cdpFrame struct {
	ID     int64           `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type cdpConn struct {
	ws      *websocket.Conn
	wmu     sync.Mutex
	nextID  int64
	pmu     sync.Mutex
	pending map[int64]chan cdpReply
	onEvent func(method string, params json.RawMessage)
	dead    chan struct{}
}

func cdpDial(wsURL string, onEvent func(string, json.RawMessage)) (*cdpConn, error) {
	cfg, err := websocket.NewConfig(wsURL, "http://localhost")
	if err != nil {
		return nil, err
	}
	ws, err := websocket.DialConfig(cfg)
	if err != nil {
		return nil, err
	}
	c := &cdpConn{ws: ws, pending: map[int64]chan cdpReply{}, onEvent: onEvent, dead: make(chan struct{})}
	go c.readLoop()
	return c, nil
}

func (c *cdpConn) readLoop() {
	for {
		var data []byte
		if err := websocket.Message.Receive(c.ws, &data); err != nil {
			break
		}
		var f cdpFrame
		if json.Unmarshal(data, &f) != nil {
			continue
		}
		if f.ID != 0 {
			c.pmu.Lock()
			ch := c.pending[f.ID]
			delete(c.pending, f.ID)
			c.pmu.Unlock()
			if ch != nil {
				r := cdpReply{result: f.Result}
				if f.Error != nil {
					r.err = errors.New(f.Error.Message)
				}
				ch <- r
			}
			continue
		}
		if f.Method != "" && c.onEvent != nil {
			c.onEvent(f.Method, f.Params)
		}
	}
	c.pmu.Lock()
	close(c.dead)
	for id, ch := range c.pending {
		ch <- cdpReply{err: errBrLiveGone}
		delete(c.pending, id)
	}
	c.pmu.Unlock()
}

func (c *cdpConn) call(ctx context.Context, method string, params, out any) error {
	id := atomic.AddInt64(&c.nextID, 1)
	ch := make(chan cdpReply, 1)
	c.pmu.Lock()
	select {
	case <-c.dead:
		c.pmu.Unlock()
		return errBrLiveGone
	default:
	}
	c.pending[id] = ch
	c.pmu.Unlock()
	drop := func() {
		c.pmu.Lock()
		delete(c.pending, id)
		c.pmu.Unlock()
	}
	body, err := json.Marshal(map[string]any{"id": id, "method": method, "params": params})
	if err != nil {
		drop()
		return err
	}
	c.wmu.Lock()
	err = websocket.Message.Send(c.ws, string(body))
	c.wmu.Unlock()
	if err != nil {
		drop()
		return err
	}
	select {
	case r := <-ch:
		if r.err != nil {
			return r.err
		}
		if out != nil && len(r.result) > 0 {
			return json.Unmarshal(r.result, out)
		}
		return nil
	case <-ctx.Done():
		drop()
		return ctx.Err()
	case <-c.dead:
		return errBrLiveGone
	}
}

func (c *cdpConn) close() { _ = c.ws.Close() }

// ---- Chrome 실행 ----

type brChrome struct {
	cmd       *exec.Cmd
	dir       string
	c         *cdpConn
	exited    chan struct{}
	loading   atomic.Bool
	mainFrame atomic.Value
	closeOnce sync.Once
}

func brOpenChrome(ctx context.Context, exe string, cols, rows int) (*brChrome, error) {
	go brSweepStale()
	dir, err := os.MkdirTemp("", "tui-live-")
	if err != nil {
		return nil, err
	}
	args := []string{
		"--headless=new", "--disable-gpu", "--disable-extensions", "--disable-sync", "--no-first-run",
		"--no-default-browser-check", "--mute-audio", "--hide-scrollbars", "--lang=ko-KR",
		"--remote-debugging-port=0", "--remote-allow-origins=http://localhost",
		"--user-data-dir=" + dir, "about:blank",
	}
	cmd := exec.Command(exe, args...)
	brPrepCmd(cmd)
	if err := cmd.Start(); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	brBindLife(cmd)
	b := &brChrome{cmd: cmd, dir: dir, exited: make(chan struct{})}
	go func() { _ = cmd.Wait(); close(b.exited) }()
	ctx, cancel := context.WithTimeout(ctx, brOpenTimeout)
	defer cancel()
	if err := b.connect(ctx, cols, rows); err != nil {
		b.Close()
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil, errBrCanceled
		}
		return nil, fmt.Errorf("브라우저를 제어하지 못함: %s", brClip(brClean(err.Error()), 100))
	}
	return b, nil
}

func (b *brChrome) connect(ctx context.Context, cols, rows int) error {
	port, err := brWaitPort(ctx, b.dir, b.exited)
	if err != nil {
		return err
	}
	wsURL, err := brPageSocket(ctx, port)
	if err != nil {
		return err
	}
	if b.c, err = cdpDial(wsURL, b.onEvent); err != nil {
		return err
	}
	if err := b.c.call(ctx, "Page.enable", nil, nil); err != nil {
		return err
	}
	var tree struct {
		FrameTree struct {
			Frame struct {
				ID string `json:"id"`
			} `json:"frame"`
		} `json:"frameTree"`
	}
	if err := b.c.call(ctx, "Page.getFrameTree", nil, &tree); err == nil {
		b.mainFrame.Store(tree.FrameTree.Frame.ID)
	}
	_ = b.c.call(ctx, "Browser.setDownloadBehavior", map[string]any{"behavior": "deny"}, nil)
	_, _ = b.addScript(ctx, brPageGuardJS)
	return b.Resize(ctx, cols, rows)
}

func (b *brChrome) addScript(ctx context.Context, src string) (string, error) {
	var r struct {
		ID string `json:"identifier"`
	}
	err := b.c.call(ctx, "Page.addScriptToEvaluateOnNewDocument", map[string]any{"source": src}, &r)
	return r.ID, err
}

// 새 창은 같은 탭으로 열고, 한글이 낱말 중간에서 줄바꿈되지 않게 하는 스크립트
const brPageGuardJS = `(()=>{try{window.open=function(u){if(u){location.href=u}return null};document.addEventListener('click',function(e){var a=e.target&&e.target.closest&&e.target.closest('a[target],form[target]');if(a)a.target='_self'},true);document.addEventListener('DOMContentLoaded',function(){var s=document.createElement('style');s.textContent='html{word-break:keep-all}';(document.head||document.documentElement).appendChild(s)})}catch(_){}})()`

func brWaitPort(ctx context.Context, dir string, exited <-chan struct{}) (int, error) {
	file := filepath.Join(dir, "DevToolsActivePort")
	for {
		if data, err := os.ReadFile(file); err == nil {
			if f := strings.Fields(string(data)); len(f) > 0 {
				if p, err := strconv.Atoi(f[0]); err == nil && p > 0 {
					return p, nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-exited:
			return 0, errors.New("브라우저가 바로 종료됨")
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func brPageSocket(ctx context.Context, port int) (string, error) {
	cl := &http.Client{Timeout: 3 * time.Second}
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(port)+"/json/list", nil)
		if err != nil {
			return "", err
		}
		if resp, err := cl.Do(req); err == nil {
			var list []struct {
				Type string `json:"type"`
				WS   string `json:"webSocketDebuggerUrl"`
			}
			err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&list)
			resp.Body.Close()
			if err == nil {
				for _, t := range list {
					if t.Type == "page" && t.WS != "" {
						return t.WS, nil
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (b *brChrome) onEvent(method string, params json.RawMessage) {
	switch method {
	case "Page.frameStartedLoading", "Page.frameStoppedLoading":
		var p struct {
			FrameID string `json:"frameId"`
		}
		if json.Unmarshal(params, &p) != nil {
			return
		}
		if mf, _ := b.mainFrame.Load().(string); mf == "" || mf == p.FrameID {
			b.loading.Store(method == "Page.frameStartedLoading")
		}
	case "Page.frameNavigated":
		var p struct {
			Frame struct {
				ID       string `json:"id"`
				ParentID string `json:"parentId"`
				URL      string `json:"url"`
			} `json:"frame"`
		}
		if json.Unmarshal(params, &p) != nil || p.Frame.ParentID != "" {
			return
		}
		b.mainFrame.Store(p.Frame.ID)
		if !brLiveURLOK(p.Frame.URL) {
			go b.c.call(context.Background(), "Page.navigate", map[string]any{"url": "about:blank"}, nil)
		}
	case "Page.javascriptDialogOpening":
		var p struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(params, &p)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = b.c.call(ctx, "Page.handleJavaScriptDialog", map[string]any{"accept": p.Type == "beforeunload"}, nil)
		}()
	}
}

// brLiveURLOK 는 라이브 세션이 머물러도 되는 주소인지 (http, https, 빈 페이지만)
func brLiveURLOK(raw string) bool {
	if raw == "about:blank" || raw == "" {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	sc := strings.ToLower(u.Scheme)
	return sc == "http" || sc == "https"
}

func (b *brChrome) Close() {
	b.closeOnce.Do(func() {
		if b.c != nil {
			b.c.close()
		}
		brKillTree(b.cmd)
		go func() {
			<-b.exited
			for i := 0; i < 10; i++ {
				if os.RemoveAll(b.dir) == nil {
					return
				}
				time.Sleep(300 * time.Millisecond)
			}
		}()
	})
}

// ---- 조작 ----

func brSleep(ctx context.Context, d time.Duration) error {
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// settle 은 동작 뒤 페이지 이동과 스크립트가 잠잠해질 때까지 잠깐 기다림
func (b *brChrome) settle(ctx context.Context) error {
	if err := brSleep(ctx, 250*time.Millisecond); err != nil {
		return err
	}
	deadline := time.Now().Add(brSettleMax)
	for b.loading.Load() && time.Now().Before(deadline) {
		if err := brSleep(ctx, 100*time.Millisecond); err != nil {
			return err
		}
	}
	return brSleep(ctx, 300*time.Millisecond)
}

func (b *brChrome) Resize(ctx context.Context, cols, rows int) error {
	return b.c.call(ctx, "Emulation.setDeviceMetricsOverride", map[string]any{
		"width": cols * brCellW, "height": rows * brCellH, "deviceScaleFactor": 1, "mobile": false,
	}, nil)
}

func (b *brChrome) Navigate(ctx context.Context, rawURL string) error {
	var r struct {
		ErrorText string `json:"errorText"`
	}
	b.loading.Store(true)
	if err := b.c.call(ctx, "Page.navigate", map[string]any{"url": rawURL}, &r); err != nil {
		return err
	}
	if r.ErrorText != "" {
		return fmt.Errorf("페이지를 열 수 없음 (%s)", brClip(brClean(r.ErrorText), 60))
	}
	return b.settle(ctx)
}

func (b *brChrome) eval(ctx context.Context, expr string, out any) error {
	var r struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		Exc json.RawMessage `json:"exceptionDetails"`
	}
	if err := b.c.call(ctx, "Runtime.evaluate", map[string]any{"expression": expr, "returnByValue": true}, &r); err != nil {
		return err
	}
	if len(r.Exc) > 0 {
		return errors.New("페이지 스크립트 오류")
	}
	if out != nil && len(r.Result.Value) > 0 {
		return json.Unmarshal(r.Result.Value, out)
	}
	return nil
}

func (b *brChrome) Snapshot(ctx context.Context) (*brSnap, error) {
	var raw string
	if err := b.eval(ctx, brSnapJS, &raw); err != nil {
		return nil, err
	}
	return brParseSnap(raw)
}

func (b *brChrome) mouse(ctx context.Context, typ string, x, y float64, extra map[string]any) error {
	p := map[string]any{"type": typ, "x": x, "y": y}
	for k, v := range extra {
		p[k] = v
	}
	return b.c.call(ctx, "Input.dispatchMouseEvent", p, nil)
}

func (b *brChrome) Click(ctx context.Context, x, y float64) error {
	if err := b.mouse(ctx, "mouseMoved", x, y, nil); err != nil {
		return err
	}
	btn := map[string]any{"button": "left", "clickCount": 1}
	if err := b.mouse(ctx, "mousePressed", x, y, btn); err != nil {
		return err
	}
	if err := b.mouse(ctx, "mouseReleased", x, y, btn); err != nil {
		return err
	}
	return b.settle(ctx)
}

func (b *brChrome) InsertText(ctx context.Context, s string) error {
	if err := b.c.call(ctx, "Input.insertText", map[string]any{"text": s}, nil); err != nil {
		return err
	}
	return brSleep(ctx, 40*time.Millisecond)
}

var brKeyTable = map[string]struct {
	key, code, text string
	vk              int
}{
	"enter":     {"Enter", "Enter", "\r", 13},
	"backspace": {"Backspace", "Backspace", "", 8},
	"delete":    {"Delete", "Delete", "", 46},
	"left":      {"ArrowLeft", "ArrowLeft", "", 37},
	"right":     {"ArrowRight", "ArrowRight", "", 39},
	"up":        {"ArrowUp", "ArrowUp", "", 38},
	"down":      {"ArrowDown", "ArrowDown", "", 40},
	"home":      {"Home", "Home", "", 36},
	"end":       {"End", "End", "", 35},
}

func (b *brChrome) Key(ctx context.Context, name string) error {
	k, ok := brKeyTable[name]
	if !ok {
		return nil
	}
	down := map[string]any{"type": "keyDown", "key": k.key, "code": k.code, "windowsVirtualKeyCode": k.vk, "nativeVirtualKeyCode": k.vk}
	if k.text != "" {
		down["text"] = k.text
	}
	if err := b.c.call(ctx, "Input.dispatchKeyEvent", down, nil); err != nil {
		return err
	}
	up := map[string]any{"type": "keyUp", "key": k.key, "code": k.code, "windowsVirtualKeyCode": k.vk, "nativeVirtualKeyCode": k.vk}
	if err := b.c.call(ctx, "Input.dispatchKeyEvent", up, nil); err != nil {
		return err
	}
	if name == "enter" {
		return b.settle(ctx)
	}
	return brSleep(ctx, 40*time.Millisecond)
}

func (b *brChrome) Scroll(ctx context.Context, x, y, dy float64) error {
	if err := b.mouse(ctx, "mouseWheel", x, y, map[string]any{"deltaX": 0, "deltaY": dy}); err != nil {
		return err
	}
	return brSleep(ctx, 250*time.Millisecond)
}

func (b *brChrome) ScrollTo(ctx context.Context, top bool) error {
	y := "0"
	if !top {
		y = "1e9"
	}
	if err := b.eval(ctx, "(document.scrollingElement||document.documentElement).scrollTo(0,"+y+")", nil); err != nil {
		return err
	}
	return brSleep(ctx, 200*time.Millisecond)
}

func (b *brChrome) SelectNext(ctx context.Context, idx int) error {
	js := "(()=>{const e=window.__tui&&window.__tui[" + strconv.Itoa(idx) + "];if(!e||e.tagName!=='SELECT'||!e.options.length)return;" +
		"e.selectedIndex=(e.selectedIndex+1)%e.options.length;e.dispatchEvent(new Event('input',{bubbles:true}));e.dispatchEvent(new Event('change',{bubbles:true}))})()"
	if err := b.eval(ctx, js, nil); err != nil {
		return err
	}
	return b.settle(ctx)
}

func (b *brChrome) History(ctx context.Context, back bool) error {
	expr := "history.forward()"
	if back {
		expr = "history.back()"
	}
	if err := b.eval(ctx, expr, nil); err != nil {
		return err
	}
	return b.settle(ctx)
}

func (b *brChrome) Reload(ctx context.Context) error {
	b.loading.Store(true)
	if err := b.c.call(ctx, "Page.reload", nil, nil); err != nil {
		return err
	}
	return b.settle(ctx)
}

var brSweepOnce sync.Once

// brSweepStale 은 프로그램이 갑자기 꺼져서 남은 오래된 임시 프로필 폴더를 지움 (쓰는 중인 폴더는 지워지지 않음)
func brSweepStale() {
	brSweepOnce.Do(func() {
		ents, err := os.ReadDir(os.TempDir())
		if err != nil {
			return
		}
		for _, e := range ents {
			if !e.IsDir() || !strings.HasPrefix(e.Name(), "tui-live-") {
				continue
			}
			if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > time.Hour {
				_ = os.RemoveAll(filepath.Join(os.TempDir(), e.Name()))
			}
		}
	})
}
