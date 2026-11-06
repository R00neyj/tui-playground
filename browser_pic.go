package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	_ "image/jpeg"
	"math"
	"strings"

	"github.com/muesli/termenv"
)

// 이미지 칸 그림: 화면 캡처를 칸마다 평균 색으로 줄여 위·아래 반 칸(▀)으로 그림.

const (
	brPicMaxRows  = 60
	brPicRowPx    = brCellH
	brShotMaxData = 8 << 20
)

// brPic 은 페이지 위 이미지 한 장의 보이는 영역 (뷰포트 픽셀)
type brPic struct {
	x, w, y0, y1 float64
	rows         int
}

func brPicRows(th float64) int {
	return min(brPicMaxRows, max(1, int(math.Round(th/brPicRowPx))))
}

// band 는 i 번째 칸 줄이 맡는 세로 구간
func (p *brPic) band(i int) (float64, float64) {
	h := (p.y1 - p.y0) / float64(p.rows)
	return p.y0 + float64(i)*h, p.y0 + float64(i+1)*h
}

// capture 는 보이는 화면을 JPEG 로 받아 디코딩함 (실패하면 nil)
func (b *brChrome) capture(ctx context.Context, vw, vh float64) image.Image {
	if vw < 1 || vh < 1 || vw > 8000 || vh > 8000 {
		return nil
	}
	var r struct {
		Data string `json:"data"`
	}
	p := map[string]any{"format": "jpeg", "quality": 55, "clip": map[string]any{"x": 0, "y": 0, "width": vw, "height": vh, "scale": 1}}
	if err := b.c.call(ctx, "Page.captureScreenshot", p, &r); err != nil || len(r.Data) == 0 || len(r.Data) > brShotMaxData {
		return nil
	}
	raw, err := base64.StdEncoding.DecodeString(r.Data)
	if err != nil {
		return nil
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil
	}
	return img
}

func brHasPics(s *brSnap) bool {
	for _, t := range s.Texts {
		if t.K == 1 {
			return true
		}
	}
	return false
}

// brAvg 는 캡처의 한 구간 평균 색 (3x3 점 표본)
func brAvg(img image.Image, sx, sy, x0, x1, y0, y1 float64) (r, g, b float64) {
	bd := img.Bounds()
	n := 0.0
	for j := 0; j < 3; j++ {
		for i := 0; i < 3; i++ {
			x := int((x0 + (x1-x0)*(float64(i)+0.5)/3) * sx)
			y := int((y0 + (y1-y0)*(float64(j)+0.5)/3) * sy)
			x, y = max(bd.Min.X, min(bd.Max.X-1, x)), max(bd.Min.Y, min(bd.Max.Y-1, y))
			cr, cg, cb, _ := img.At(x, y).RGBA()
			r, g, b, n = r+float64(cr>>8), g+float64(cg>>8), b+float64(cb>>8), n+1
		}
	}
	return r / n, g / n, b / n
}

// brPicCells 는 이미지 항목 하나를 (글자, 앞색, 뒷색) 칸들로 바꿈
func brPicCells(s *brSnap, it brItem) []brCell {
	w := it.width()
	out := make([]brCell, 0, w)
	if s == nil || s.Shot == nil || it.pic == nil || w == 0 {
		return out
	}
	sx, sy := 1.0, 1.0
	if s.VW > 0 && s.VH > 0 {
		sx, sy = float64(s.Shot.Bounds().Dx())/s.VW, float64(s.Shot.Bounds().Dy())/s.VH
	}
	p := it.pic
	y0, y1 := p.band(it.band)
	ym := (y0 + y1) / 2
	ascii := termProfile() == termenv.Ascii
	for i := 0; i < w; i++ {
		x0, x1 := p.x+float64(i)*p.w/float64(w), p.x+float64(i+1)*p.w/float64(w)
		tr, tg, tb := brAvg(s.Shot, sx, sy, x0, x1, y0, ym)
		br, bg, bb := brAvg(s.Shot, sx, sy, x0, x1, ym, y1)
		c := brCell{s: "▀", w: 1, fg: colorPicture(tr, tg, tb), bg: colorPicture(br, bg, bb), elem: it.elem}
		if ascii {
			l := (0.3*(tr+br) + 0.59*(tg+bg) + 0.11*(tb+bb)) / 2 / 255
			c = brCell{s: string(" .:-=+*#%@"[int(math.Min(9, l*10))]), w: 1, fg: colorText, elem: it.elem}
		}
		out = append(out, c)
	}
	return out
}

func brSpaces(n int) string { return strings.Repeat(" ", max(0, n)) }

// brPicCand 는 이미지 글자 항목을 그림 후보로 바꿈 (보이는 폭이 없으면 false)
func brPicCand(s *brSnap, t brSnapText, cols int) (brCand, bool) {
	th := t.TH
	if th <= 0 {
		th = t.H
	}
	vw := s.VW
	if vw <= 0 {
		vw = float64(s.Shot.Bounds().Dx())
	}
	x0, x1 := math.Max(0, t.X), math.Min(vw, t.X+t.W)
	if x1-x0 < 1 || th < 1 {
		return brCand{}, false
	}
	p := &brPic{x: x0, w: x1 - x0, y0: t.Y, y1: t.Y + th, rows: brPicRows(th)}
	wc := max(1, min(cols-1, int(math.Round(p.w/brCellW))))
	return brCand{x: x0, y: t.Y, w: p.w, h: t.H, text: brSpaces(wc), kind: biPic, elem: t.F, pic: p, ext: th}, true
}

// brPicRowsBelow 는 줄에 놓인 이미지의 둘째 줄부터를 이어 붙이고 다음 줄 번호를 돌려줌
func brPicRowsBelow(sc *brScreen, main []brItem, row int) int {
	most := 0
	for _, it := range main {
		if it.pic != nil {
			most = max(most, it.pic.rows-1)
		}
	}
	for i := 1; i <= most; i++ {
		var r []brItem
		ry := 0.0
		for _, it := range main {
			if it.pic != nil && it.pic.rows > i {
				r = append(r, brItem{col: it.col, text: it.text, kind: biPic, elem: it.elem, pic: it.pic, band: i})
				if ry == 0 {
					y0, y1 := it.pic.band(i)
					ry = (y0 + y1) / 2
				}
			}
		}
		sc.rows = append(sc.rows, r)
		sc.rowY = append(sc.rowY, ry)
		row++
	}
	return row
}
