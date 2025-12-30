package certificate

import (
	"strings"
	"time"

	"crypto/rand"
	"math/big"
)

// calculateBackoffWithJitter calculates exponential backoff with jitter for retry attempts
// Returns duration for backoff: attempt^2 seconds + random jitter (0-25% of backoff)
func calculateBackoffWithJitter(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}

	backoff := time.Duration(attempt*attempt) * time.Second

	jitterMax := int64(backoff / 4)
	var jitter time.Duration
	if jitterMax > 0 {
		n, err := rand.Int(rand.Reader, big.NewInt(jitterMax))
		if err == nil {
			jitter = time.Duration(n.Int64())
		}
	}

	return backoff + jitter
}

// calculateRateLimitAwareBackoff calculates backoff that respects Let's Encrypt rate limits
// Let's Encrypt allows 5 authorization failures per hour (refills 1 every 12 minutes)
func calculateRateLimitAwareBackoff(attempt int, isProduction bool) time.Duration {
	if attempt <= 0 {
		return 0
	}

	if !isProduction {
		// Staging environment is more generous (200 failures/hour)
		backoff := time.Duration(1<<uint(attempt-1)) * time.Minute
		if backoff > 30*time.Minute {
			backoff = 30 * time.Minute
		}

		// Add 20% jitter using crypto/rand
		jitterMax := int64(backoff / 5)
		var jitter time.Duration
		if jitterMax > 0 {
			n, err := rand.Int(rand.Reader, big.NewInt(jitterMax))
			if err == nil {
				jitter = time.Duration(n.Int64())
			}
		}

		return backoff + jitter
	}

	// Production environment: respect 5 failures/hour limit
	switch attempt {
	case 1:
		return 2 * time.Minute
	case 2:
		return 12 * time.Minute
	case 3:
		return 15 * time.Minute
	case 4:
		return 20 * time.Minute
	default:
		return 30 * time.Minute
	}
}

// isRateLimitError checks if an error indicates Let's Encrypt rate limiting
func isRateLimitError(err error) bool {
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "rate limit") ||
		strings.Contains(errStr, "too many certificates") ||
		strings.Contains(errStr, "rate limited") ||
		strings.Contains(errStr, "ratelimited")
}

