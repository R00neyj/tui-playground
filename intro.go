package main

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// 인트로 단계별 시각(시작 기준)
const (
	introLogoEnd  = 700 * time.Millisecond
	introSubEnd   = 1150 * time.Millisecond
	introBarEnd   = 1950 * time.Millisecond
	introTotal    = 2350 * time.Millisecond
	logoChunks    = 18
	introBarWidth = 40
)

// 로고는 영문 약칭 "TUI" (한글 앱 이름은 부제목으로 표시)
var logoRows = []string{
	"████████╗██╗   ██╗██╗",
	"╚══██╔══╝██║   ██║██║",
	"   ██║   ██║   ██║██║",
	"   ██║   ██║   ██║██║",
	"   ██║   ╚██████╔╝██║",
	"   ╚═╝    ╚═════╝ ╚═╝",
}

const appName = "터미널 플레이그라운드"

var logoGrad = func() []lipgloss.Color {
	var g []lipgloss.Color
	for _, p := range []struct {
		c lipgloss.Color
		n int
	}{{cBlue, 3}, {cDarkCyan, 3}, {cCyan, 6}, {cWhite, 3}, {cGray, 3}} {
		for i := 0; i < p.n; i++ {
			g = append(g, p.c)
		}
	}
	return g
}()

var introSteps = []string{"시스템 정보 수집", "예약 상태 확인", "화면 구성"}

func (m model) viewIntro() string {
	t := time.Since(m.introStart)
	rows := logoRows
	width := 0
	for _, r := range rows {
		width = max(width, lipgloss.Width(r))
	}

	// 로고: 왼쪽에서 오른쪽으로 흰색 빛이 지나가며 그라데이션 색으로 자리잡음
	chunk := int(math.Ceil(float64(width) / logoChunks))
	used := int(math.Ceil(float64(width) / float64(chunk)))
	sweep := int(float64(t) / float64(introLogoEnd) * float64(used+3))
	if t >= introLogoEnd {
		sweep = used + 3
	}
	var out []string
	for _, r := range rows {
		rs := []rune(r)
		for len(rs) < width {
			rs = append(rs, ' ')
		}
		var b strings.Builder
		for i := 0; i < used; i++ {
			s, e := i*chunk, min(len(rs), (i+1)*chunk)
			part := string(rs[s:e])
			switch {
			case i > sweep:
				b.WriteString(strings.Repeat(" ", e-s))
			case sweep-i < 3:
				b.WriteString(fg(part, cWhite))
			default:
				b.WriteString(fg(part, logoGrad[i*len(logoGrad)/used]))
			}
		}
		out = append(out, b.String())
	}
	out = append(out, "")

	// 부제목(앱 이름): 한 글자씩 타이핑. 글자 사이를 띄우고 단어 사이는 3칸
	sub := []rune(strings.Join(strings.Split(strings.ReplaceAll(appName, " ", "\x00"), ""), " "))
	subStr := strings.ReplaceAll(string(sub), " \x00 ", "   ")
	sub = []rune(subStr)
	n := 0
	if t >= introLogoEnd {
		n = len(sub)
		if t < introSubEnd {
			n = int(float64(t-introLogoEnd) / float64(introSubEnd-introLogoEnd) * float64(len(sub)))
		}
	}
	typed := string(sub[:n])
	out = append(out, fg(typed, cDarkGray)+strings.Repeat(" ", lipgloss.Width(subStr)-lipgloss.Width(typed)), "")

	// 진행 막대
	p := 0
	if t >= introSubEnd {
		p = 100
		if t < introBarEnd {
			p = int(float64(t-introSubEnd) / float64(introBarEnd-introSubEnd) * 100)
		}
	}
	filled := introBarWidth * p / 100
	if t >= introSubEnd {
		out = append(out, fg(strings.Repeat("█", filled), cCyan)+fg(strings.Repeat("░", introBarWidth-filled), cDarkGray)+fg(fmt.Sprintf(" %3d%%", p), cWhite))
		if t >= introBarEnd {
			out = append(out, fg("준비 완료", cGreen))
		} else {
			out = append(out, fg(introSteps[min(2, p/34)], cDarkGray))
		}
	}

	// 줄마다 가운데 정렬 (줄 길이가 달라도 로고 위치가 흔들리지 않도록 로고 폭 기준 블록)
	block := lipgloss.NewStyle().Width(max(width, introBarWidth+5)).Align(lipgloss.Center).Render(strings.Join(out, "\n"))
	return m.center(block)
}
