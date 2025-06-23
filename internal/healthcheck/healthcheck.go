package healthcheck

import "time"

// Checker is the common interface implemented by all component health checkers.
type Checker interface {
	// Name returns the component name.
	Name() string
	// CheckOnce performs a single health-probe. ok==true means healthy; latency is the
	// time it took; err is populated on failure.
	CheckOnce() (ok bool, latency time.Duration, err error)
	// WaitHealthy blocks until a probe succeeds or retries/timeouts are exhausted.
	WaitHealthy() bool
}

// Result holds the outcome of a health probe.
type Result struct {
	Healthy bool
	Latency time.Duration
	Error   error
}

// Aggregate runs each checker once and returns per-component results plus an overall flag.
func Aggregate(checkers ...Checker) (map[string]Result, bool) {
	results := make(map[string]Result, len(checkers))
	allHealthy := true
	for _, chk := range checkers {
		ok, dur, err := chk.CheckOnce()
		if !ok {
			allHealthy = false
		}
		results[chk.Name()] = Result{Healthy: ok, Latency: dur, Error: err}
	}
	return results, allHealthy
}
