//go:build !windows

package main

import "time"

// Windows 가 아니면 소리를 내지 않음 (터미널 벨은 음높이를 바꿀 수 없음). 건반 불빛만 보임.

const pianoHasSound = false

func platformTone(freq float64, d time.Duration) {}
