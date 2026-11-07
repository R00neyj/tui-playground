package main

import (
	"encoding/binary"
	"math"
	"time"
)

// 피아노 소리 합성. 터미널은 키를 뗀 순간을 알려주지 않으므로 "누르고 있는 동안" 소리를 낼 수 없음.
// 대신 음마다 정해진 길이(pianoLife) 동안 잦아들며 울리게 하고, 새 음을 누르면 아직 울리는 음들과 섞은 소리를 새로 만들어 틂.
// 그래서 여러 키를 거의 동시에 누르면 화음이 되고, 이어서 누르면 앞 음의 여운 위에 새 음이 겹침.

const (
	pianoRate = 22050
	pianoLife = 900 * time.Millisecond // 음 하나가 울리는 길이
	pianoMaxN = 10                     // 동시에 섞는 음 상한 (오래된 음부터 뺌)
)

type pianoNote struct {
	freq  float64
	start time.Time
}

// pianoMixer 는 아직 울리고 있는 음 목록
type pianoMixer struct{ notes []pianoNote }

// pianoVoice 는 음 하나의 t초 시점 값 (-1..1 근처). 기본음 + 2·3배음, 짧은 상승 뒤 지수 감쇠, 끝은 0 으로 수렴
func pianoVoice(freq, t float64) float64 {
	life := pianoLife.Seconds()
	if t < 0 || t >= life {
		return 0
	}
	env := math.Exp(-t * 4)
	if a := 0.005; t < a {
		env *= t / a
	}
	if tail := life - t; tail < 0.05 {
		env *= tail / 0.05
	}
	w := 2 * math.Pi * freq * t
	return (math.Sin(w) + 0.4*math.Sin(2*w) + 0.15*math.Sin(3*w)) / 1.55 * env
}

// press 는 now 에 freq 음을 추가하고, 지금부터 울릴 모든 음을 섞은 WAV 를 돌려줌
func (m *pianoMixer) press(freq float64, now time.Time) []byte {
	live := m.notes[:0]
	for _, n := range m.notes {
		if now.Sub(n.start) < pianoLife {
			live = append(live, n)
		}
	}
	m.notes = append(live, pianoNote{freq, now})
	if len(m.notes) > pianoMaxN {
		m.notes = m.notes[len(m.notes)-pianoMaxN:]
	}
	n := int(pianoLife.Seconds() * pianoRate)
	data := wavHeader(n)
	offs := make([]float64, len(m.notes))
	for k, nt := range m.notes {
		offs[k] = now.Sub(nt.start).Seconds() // 앞 음은 이어서 울리도록 지난 시간만큼 앞에서 시작
	}
	for i := 0; i < n; i++ {
		t := float64(i) / pianoRate
		sum := 0.0
		for k, nt := range m.notes {
			sum += pianoVoice(nt.freq, offs[k]+t)
		}
		v := math.Tanh(sum*0.8) * 0.7 // 여러 음이 겹쳐도 소리가 찢어지지 않게 부드럽게 눌러 줌
		binary.LittleEndian.PutUint16(data[44+i*2:], uint16(int16(v*32767)))
	}
	return data
}

// wavHeader 는 샘플 n 개(16비트 모노)가 들어갈 WAV 파일 버퍼를 머리까지 채워서 돌려줌
func wavHeader(n int) []byte {
	data := make([]byte, 44+n*2)
	put32 := func(off int, v uint32) { binary.LittleEndian.PutUint32(data[off:], v) }
	put16 := func(off int, v uint16) { binary.LittleEndian.PutUint16(data[off:], v) }
	copy(data[0:], "RIFF")
	put32(4, uint32(36+n*2))
	copy(data[8:], "WAVE")
	copy(data[12:], "fmt ")
	put32(16, 16)
	put16(20, 1) // PCM
	put16(22, 1) // 모노
	put32(24, pianoRate)
	put32(28, pianoRate*2)
	put16(32, 2)
	put16(34, 16)
	copy(data[36:], "data")
	put32(40, uint32(n*2))
	return data
}
