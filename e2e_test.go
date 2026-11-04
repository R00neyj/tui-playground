package main

import (
	"bytes"
	"io"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}
func (l *lockedBuf) String() string { l.mu.Lock(); defer l.mu.Unlock(); return l.b.String() }

// 실제 Bubble Tea 이벤트 루프에 키 입력을 흘려 넣어 인트로 건너뛰기 → 앱마다 열기 → Esc 로 홈 복귀 → 종료까지 돌려봄.
// 사용자 기록 파일(Claude/Codex)과 할 일 파일은 임시 폴더로 돌려서 실제 데이터를 읽거나 쓰지 않음.
func TestProgramEndToEnd(t *testing.T) {
	tmp := t.TempDir()
	for _, k := range []string{"LOCALAPPDATA", "APPDATA", "USERPROFILE", "HOME", "XDG_CONFIG_HOME", "XDG_CACHE_HOME"} {
		t.Setenv(k, tmp)
	}
	pr, pw := io.Pipe()
	defer pw.Close()
	out := &lockedBuf{}
	p := tea.NewProgram(newModel(), tea.WithInput(pr), tea.WithOutput(out), tea.WithoutSignals())
	done := make(chan error, 1)
	go func() { _, err := p.Run(); done <- err }()

	send := func(s string) {
		if _, err := pw.Write([]byte(s)); err != nil {
			t.Fatalf("키 입력 실패: %v", err)
		}
		time.Sleep(250 * time.Millisecond)
	}
	p.Send(tea.WindowSizeMsg{Width: 100, Height: 30})
	time.Sleep(300 * time.Millisecond)
	send("x") // 인트로 건너뛰기
	if !strings.Contains(plain(out.String()), registry[0].Name) {
		t.Fatalf("홈 화면에 앱 이름이 보이지 않음")
	}
	for i, info := range registry {
		send(strconv.Itoa(i + 1)) // 숫자키로 열기
		time.Sleep(300 * time.Millisecond)
		send("\x1b") // Esc — 처음 상태에서는 바로 홈으로 돌아와야 함
		time.Sleep(150 * time.Millisecond)
		select {
		case err := <-done:
			t.Fatalf("%s: Esc 이후 프로그램이 끝나 버림 (err=%v)", info.ID, err)
		default:
		}
	}
	send("q") // 홈에서 종료
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("프로그램이 오류로 끝남: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("홈에서 q 를 눌러도 프로그램이 끝나지 않음")
	}
}
