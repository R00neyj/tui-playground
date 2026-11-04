package main

// 자리표시 구현. 이 파일을 통째로 실제 앱으로 바꿔서 쓴다 (AppInfo 변수 이름은 유지할 것).
var sysmonInfo = AppInfo{
	ID:   "sysmon",
	Name: "시스템 모니터",
	Desc: "CPU·메모리·디스크 실시간",
	New:  func(env *Env) App { return newStub("시스템 모니터") },
}
