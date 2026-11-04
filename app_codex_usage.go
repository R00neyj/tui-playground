package main

// Codex 사용량 (F-09). 화면과 읽기 로직은 usage_view.go, 기록 형식은 usage_codex.go 에 있음.
var codexUsageInfo = AppInfo{
	ID:   "codex-usage",
	Name: "Codex 사용량",
	Desc: "Codex 토큰 사용 기록",
	New:  func(env *Env) App { return newUsageApp(codexUsageSource) },
}
