//go:build windows

package cli

import (
	"syscall"
	"time"
	"unsafe"
)

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	procQPC  = kernel32.NewProc("QueryPerformanceCounter")
	procQPF  = kernel32.NewProc("QueryPerformanceFrequency")
	qpfFreq  int64
)

func init() {
	_, _, _ = procQPF.Call(uintptr(unsafe.Pointer(&qpfFreq)))
}

type hrTimer struct {
	startTicks int64
	fallback   time.Time
}

func startTimer() hrTimer {
	if qpfFreq > 0 {
		var ticks int64
		_, _, _ = procQPC.Call(uintptr(unsafe.Pointer(&ticks)))
		return hrTimer{startTicks: ticks}
	}
	return hrTimer{fallback: time.Now()}
}

func (t hrTimer) Elapsed() time.Duration {
	if qpfFreq > 0 {
		var endTicks int64
		_, _, _ = procQPC.Call(uintptr(unsafe.Pointer(&endTicks)))
		delta := endTicks - t.startTicks
		if delta < 0 {
			return 0
		}
		// Converte ticks de hardware para nanossegundos sem overflow em durações curtas
		return time.Duration((delta * 1_000_000_000) / qpfFreq)
	}
	return time.Since(t.fallback)
}
