//go:build windows

package main

import (
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows 는 winmm 의 PlaySound 로 메모리에 만든 WAV 를 비동기로 재생함 (piano_wav.go 가 아직 울리는 음들을 섞어 만듦).
// 처음에는 kernel32 Beep 을 썼는데, 호출은 성공(반환값 1)해도 소리가 안 나는 PC 가 있어서 바꿈.
// PlaySound 는 새 소리를 틀면 앞 소리를 끊지만, 새 소리 안에 앞 음의 남은 부분이 섞여 있어서 이어 들림.

const pianoHasSound = true

const (
	sndAsync     = 0x0001
	sndNoDefault = 0x0002
	sndMemory    = 0x0004
)

var (
	playSoundProc = windows.NewLazySystemDLL("winmm.dll").NewProc("PlaySoundW")
	pianoMix      pianoMixer
	pianoPlaying  []byte // 비동기 재생 중에는 버퍼가 살아 있어야 하므로 붙잡아 둠
)

func platformTone(freq float64, _ time.Duration) {
	wav := pianoMix.press(freq, time.Now())
	_, _, _ = playSoundProc.Call(uintptr(unsafe.Pointer(&wav[0])), 0, sndAsync|sndNoDefault|sndMemory)
	pianoPlaying = wav // PlaySound 가 앞 소리를 멈춘 뒤에 바꿔야 앞 버퍼를 놓아도 안전함
}
