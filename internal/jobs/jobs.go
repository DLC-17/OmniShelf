// Package jobs provides the process-wide guard that prevents the nightly
// TMDB sync and CSV import jobs from running concurrently.
package jobs

import "sync/atomic"

var busy atomic.Bool

// TryLock claims the background-job slot. It returns false if another
// background job (sync or import) is already running; callers must skip
// their run and log rather than wait.
func TryLock() bool { return busy.CompareAndSwap(false, true) }

// Unlock releases the background-job slot.
func Unlock() { busy.Store(false) }
