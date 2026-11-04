package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/muesli/termenv"
)

// 홈 화면 설정: 배경 종류, 화면 전환 종류, 아케이드 조작부.
// 사용자 설정 폴더(Windows: %APPDATA%\TUI-playground\settings.json)에 JSON 으로 저장함.

type Settings struct {
	Bg     string `json:"bg"`     // "A","B","C","E","H" 또는 "" (끔)
	Trans  int    `json:"trans"`  // 1~4, 0 은 끔
	Arcade bool   `json:"arcade"` // 아케이드 조작부
}

var defaultSettings = Settings{Bg: "H", Trans: 3, Arcade: false}

// 테스트에서 바꿔 끼울 수 있게 변수로 둠
var settingsPath = func() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		return ""
	}
	return filepath.Join(dir, "TUI-playground", "settings.json")
}

// loadSettings 는 파일이 없거나 깨졌거나 값이 이상하면 기본값을 돌려줌 (값 단위로 고침)
func loadSettings() Settings {
	s := defaultSettings
	p := settingsPath()
	if p == "" {
		return s
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return s
	}
	var raw struct {
		Bg     *string `json:"bg"`
		Trans  *int    `json:"trans"`
		Arcade *bool   `json:"arcade"`
	}
	if json.Unmarshal(data, &raw) != nil {
		return s
	}
	if raw.Bg != nil && (*raw.Bg == "" || validBg(*raw.Bg)) {
		s.Bg = *raw.Bg
	}
	if raw.Trans != nil && validTrans(*raw.Trans) {
		s.Trans = *raw.Trans
	}
	if raw.Arcade != nil {
		s.Arcade = *raw.Arcade
	}
	return s
}

// save 는 임시 파일에 쓴 뒤 교체함. 실패해도 이번 실행에서는 설정이 적용된 채로 둠
func (s Settings) save() error {
	p := settingsPath()
	if p == "" {
		return fmt.Errorf("설정 폴더를 알 수 없음")
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// ---- 설정 화면 ----

const (
	setBg = iota
	setTrans
	setArcade
	setCount
)

var setLabels = [setCount]string{"홈 배경", "화면 전환", "아케이드 조작부"}

// bgOptions: 끔 + 배경 종류들
func bgLabel(id string) string {
	if id == "" {
		return "끔"
	}
	for _, k := range bgKinds {
		if k.ID == id {
			return k.ID + " " + k.Name
		}
	}
	return id
}

func transLabel(id int) string {
	if id == 0 {
		return "끔"
	}
	for _, k := range transKinds {
		if k.ID == id {
			return fmt.Sprintf("%d %s", k.ID, k.Name)
		}
	}
	return "-"
}

// cycle 은 항목 row 의 값을 dir(+1/-1) 만큼 돌림 (끝에서 반대쪽으로 이어짐)
func (s Settings) cycle(row, dir int) Settings {
	switch row {
	case setBg:
		ids := []string{""}
		for _, k := range bgKinds {
			ids = append(ids, k.ID)
		}
		i := 0
		for j, id := range ids {
			if id == s.Bg {
				i = j
			}
		}
		s.Bg = ids[(i+dir+len(ids))%len(ids)]
	case setTrans:
		n := len(transKinds) + 1
		s.Trans = (s.Trans + dir + n) % n
	case setArcade:
		s.Arcade = !s.Arcade
	}
	return s
}

func (s Settings) valueLabel(row int) string {
	switch row {
	case setBg:
		return bgLabel(s.Bg)
	case setTrans:
		return transLabel(s.Trans)
	}
	if s.Arcade {
		return "켬"
	}
	return "끔"
}

func (m model) viewSettings(c Ctx) string {
	f := NewFrame(c).Blank()
	f.Row(sgBold("설정", colorAccent))
	f.Blank()
	for i := 0; i < setCount; i++ {
		mark := "  "
		lab := sg(padR(setLabels[i], 18), colorText)
		val := sg("◀ "+m.set.valueLabel(i)+" ▶", colorTextMuted)
		if i == m.setRow {
			mark = "▸ "
			lab = sgBold(padR(setLabels[i], 18), colorAccent)
			val = sgBold("◀ "+m.set.valueLabel(i)+" ▶", colorAccent)
		}
		f.Row(sg(mark, colorAccent), lab, val)
	}
	f.Blank().Rule()
	f.MenuInline([2]string{"↑↓", "항목"}, [2]string{"←→", "바꾸기"}, [2]string{"Esc", "홈으로"})
	f.Rule().Message(m.settingsNote(c), ToneInfo)
	return f.String()
}

// settingsNote 는 지금 설정이 실제로 적용되는지 알려주는 한 줄
func (m model) settingsNote(c Ctx) string {
	switch {
	case !animEnabled():
		return "이 터미널에서는 색 효과를 쓸 수 없어 배경과 전환이 꺼져 있음"
	case termProfile() != termenv.TrueColor:
		return "트루컬러를 지원하지 않아 256색 또는 16색으로 대체해 표시함"
	case m.set.Arcade && !m.arcadeFits(c):
		return fmt.Sprintf("조작부는 창 세로 %d줄 이상일 때만 보임", m.homeBoxLines(c)+arcadeH)
	case m.saveErr != "":
		return "설정 저장 실패: " + strings.TrimSpace(m.saveErr)
	}
	return "바꾸는 즉시 뒤 화면에 적용되고 저장됨"
}
