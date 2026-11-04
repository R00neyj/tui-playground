package main

// 자리표시 구현. 이 파일을 통째로 실제 앱으로 바꿔서 쓴다 (AppInfo 변수 이름은 유지할 것).
var codexUsageInfo = AppInfo{
	ID:   "codex-usage",
	Name: "Codex 사용량",
	Desc: "Codex 토큰 사용 기록",
	New:  func(env *Env) App { return newStub("Codex 사용량") },
}
