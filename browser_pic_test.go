package main

import (
	"github.com/muesli/termenv"
	"image"
	"image/color"
	"testing"
)

func picSnap() *brSnap {
	img := image.NewRGBA(image.Rect(0, 0, 700, 360))
	for y := 0; y < 360; y++ {
		for x := 0; x < 700; x++ {
			c := color.RGBA{255, 0, 0, 255}
			if y >= 27 {
				c = color.RGBA{0, 0, 255, 255}
			}
			img.Set(x, y, c)
		}
	}
	return &brSnap{VW: 700, VH: 360, Shot: img, Texts: []brSnapText{
		{T: "[이미지]", X: 70, Y: 0, W: 140, H: 40, F: -1, K: 1, TH: 54},
		{T: "아래 글", X: 70, Y: 150, W: 60, H: 18, F: -1},
	}}
}

func TestPicLayoutReservesRowsAndSamplesColors(t *testing.T) {
	forceProfile(t, termenv.TrueColor)
	sc := brLayoutSnap(picSnap(), 90)
	if len(sc.rows) < 4 {
		t.Fatalf("rows=%d, 이미지 3줄 + 글 1줄이 필요함", len(sc.rows))
	}
	pics := 0
	for _, r := range sc.rows {
		for _, it := range r {
			if it.kind == biPic {
				pics++
				if it.width() != 20 {
					t.Errorf("폭 %d, want 20", it.width())
				}
			}
		}
	}
	if pics != 3 {
		t.Fatalf("이미지 줄=%d, want 3", pics)
	}
	first := brPicCells(sc.snap, sc.rows[0][0])
	if len(first) != 20 || first[0].s != "▀" {
		t.Fatalf("칸=%d %q", len(first), first[0].s)
	}
	if first[0].fg != colorPicture(255, 0, 0) || first[0].bg != colorPicture(255, 0, 0) {
		t.Errorf("첫 줄은 전부 빨강이어야 함: %v %v", first[0].fg, first[0].bg)
	}
	last := brPicCells(sc.snap, sc.rows[2][0])
	if last[0].bg != colorPicture(0, 0, 255) {
		t.Errorf("마지막 줄 아래쪽은 파랑이어야 함: %v", last[0].bg)
	}
}

func TestPicFallsBackToLabelWithoutShot(t *testing.T) {
	s := picSnap()
	s.Shot = nil
	sc := brLayoutSnap(s, 90)
	for _, r := range sc.rows {
		for _, it := range r {
			if it.kind == biPic {
				t.Fatalf("캡처가 없으면 이미지 항목을 만들면 안 됨")
			}
		}
	}
}

func TestPicClipsToViewportAndCols(t *testing.T) {
	s := picSnap()
	s.Texts[0].X, s.Texts[0].W = -50, 5000
	sc := brLayoutSnap(s, 40)
	for _, r := range sc.rows {
		for _, it := range r {
			if it.col+it.width() > 40 {
				t.Fatalf("폭 초과: col=%d w=%d", it.col, it.width())
			}
		}
	}
}
