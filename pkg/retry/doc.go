// Package retry provides comprehensive HTTP retry functionality with configurable
// backoff strategies, jitter, and intelligent error classification.
//
// The package implements production-ready retry logic suitable for HTTP clients
// and other network operations. It supports multiple backoff strategies including
// linear, exponential, and decorrelated exponential backoff. Jitter strategies
// help prevent thundering herd problems.
//
// Basic usage:
//
//	config := retry.DefaultRetryConfig()
//	retrier := retry.NewRetrier(config)
//
//	err := retrier.Do(ctx, func() error {
//	    resp, err := httpClient.Get(url)
//	    if err != nil {
//	        return err
//	    }
//	    return nil
//	})
//
// The package provides detailed error classification to determine which errors
// should be retried:
//   - Transient errors (network timeouts, temporary failures)
//   - Timeout errors (deadline exceeded)
//   - Unavailable errors (service unavailable)
//   - Throttled errors (rate limiting)
//   - Permanent errors (invalid input, unauthorized)
//
// All retry operations respect context cancellation, allowing for graceful
// shutdown and timeout handling.
package retry
