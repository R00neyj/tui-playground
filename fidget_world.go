package main

import "github.com/charmbracelet/lipgloss"

// 고정 크기 칸 세계(ww x wh)를 창 크기의 점자 캔버스로 옮겨 그리는 도구. 모래·생명 게임이 씀.
// 상태는 창 크기와 무관하게 세계 좌표로 두고, 그릴 때만 늘이거나 줄임.
// 캔버스 점 하나가 덮는 세계 사각형 안에 칠해진 칸이 하나라도 있으면 그 점을 찍음 (줄여도 칸이 사라지지 않음).

// worldPrio 는 한 칸(점자 글자) 안에 여러 색이 섞일 때 이기는 순서. 커서(accent)가 항상 보이게 함
func worldPrio(c lipgloss.Color) int {
	switch c {
	case colorAccent:
		return 4
	case colorWarn, colorOk:
		return 3
	case colorText:
		return 2
	case colorTextMuted:
		return 1
	}
	return 0
}

// worldRows 는 at(x, y) 가 돌려주는 색("" 이면 빈 칸)으로 세계를 그려 정확히 ch 줄을 돌려줌
func worldRows(cw, ch, ww, wh int, at func(x, y int) lipgloss.Color) [][]seg {
	cv := newCanvas(cw, ch)
	cells := make([]lipgloss.Color, cv.cw*cv.ch)
	W, H := cv.W(), cv.H()
	for y := 0; y < H; y++ {
		wy0 := y * wh / H
		wy1 := max(wy0+1, (y+1)*wh/H)
		for x := 0; x < W; x++ {
			wx0 := x * ww / W
			wx1 := max(wx0+1, (x+1)*ww/W)
			var best lipgloss.Color
			for wy := wy0; wy < wy1 && wy < wh; wy++ {
				for wx := wx0; wx < wx1 && wx < ww; wx++ {
					if c := at(wx, wy); c != "" && (best == "" || worldPrio(c) > worldPrio(best)) {
						best = c
					}
				}
			}
			if best == "" {
				continue
			}
			cv.Set(x, y)
			i := y/4*cv.cw + x/2
			if cells[i] == "" || worldPrio(best) > worldPrio(cells[i]) {
				cells[i] = best
			}
		}
	}
	rows := make([][]seg, cv.ch)
	for cy := range rows {
		rows[cy] = cv.RowFunc(cy, func(cx int) lipgloss.Color {
			if c := cells[cy*cv.cw+cx]; c != "" {
				return c
			}
			return colorText
		})
	}
	return rows[:max(0, min(ch, len(rows)))]
}
