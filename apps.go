package main

// registry 는 홈 메뉴에 나오는 앱 목록 (위에서부터 1, 2, 3 … 번).
// 새 앱은 여기에 한 줄만 추가하면 됨 (F-03).
var registry = []AppInfo{
	timerInfo,
	pomodoroInfo,
	stopwatchInfo,
	sysmonInfo,
	claudeUsageInfo,
	codexUsageInfo,
	todoInfo,
	diskInfo,
	browserInfo,
}
