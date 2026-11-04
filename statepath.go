package main

import (
	"os"
	"path/filepath"
)

// 테스트에서 바꿔 끼울 수 있게 변수로 둠
var stateUserCacheDir = os.UserCacheDir

// statePath 는 종료 타이머 상태 파일 경로.
// LOCALAPPDATA 가 설정돼 있으면 그대로 씀 (Windows 기본값이자 테스트가 쓰는 환경 변수).
// 없으면 사용자 캐시 폴더 아래 TUI-playground 폴더를 쓰고 (macOS: ~/Library/Caches, Linux: $XDG_CACHE_HOME 또는 ~/.cache),
// 그것도 못 구하면 임시 폴더를 씀.
func statePath() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		if d, err := stateUserCacheDir(); err == nil && d != "" {
			base = filepath.Join(d, "TUI-playground")
			if os.MkdirAll(base, 0o700) != nil { // 폴더를 못 만들면 임시 폴더로 대체
				base = os.TempDir()
			}
		} else {
			base = os.TempDir()
		}
	}
	return filepath.Join(base, "shutdown_timer_go.state")
}
