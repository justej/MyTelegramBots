package bot

import "time"

func RobustExecute(n int, d time.Duration, f func() bool) bool {
	for i := 0; i < n; i++ {
		if f() {
			return true
		}
		time.Sleep(d)
	}
	return false
}
