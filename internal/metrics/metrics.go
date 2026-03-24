// Package metrics holds lightweight process-wide counters for observability.
package metrics

import "sync/atomic"

var (
	providerCompleteTotal  atomic.Uint64
	providerCompleteErrors atomic.Uint64
	providerCacheHits      atomic.Uint64
)

// RecordProviderComplete increments completion count; errors increments the error counter.
func RecordProviderComplete(ok bool) {
	providerCompleteTotal.Add(1)
	if !ok {
		providerCompleteErrors.Add(1)
	}
}

// RecordProviderCacheHit increments the provider response cache hit counter.
func RecordProviderCacheHit() {
	providerCacheHits.Add(1)
}

// Snapshot returns current counter values (for tests or future export).
func Snapshot() (completeTotal, completeErrors, cacheHits uint64) {
	return providerCompleteTotal.Load(), providerCompleteErrors.Load(), providerCacheHits.Load()
}
