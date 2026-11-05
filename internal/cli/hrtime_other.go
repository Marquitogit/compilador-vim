//go:build !windows

package cli

import "time"

type hrTimer struct {
	start time.Time
}

func startTimer() hrTimer {
	return hrTimer{start: time.Now()}
}

func (t hrTimer) Elapsed() time.Duration {
	return time.Since(t.start)
}
