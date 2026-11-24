package asyncapi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/segmentio/kafka-go"
	mustererrors "github.com/wojons/muster/pkg/errors"
)

// mapKafkaError maps a transport-boundary failure onto the PROTO-007 §6
// table. Classification is by TYPED transport error (kafka protocol error
// codes, net.Error timeouts, context errors) — never message substring
// matching — and the underlying cause is preserved via mustererrors.Wrap
// for errors.IsError and verbose diagnostics.
func mapKafkaError(err error, opContext string) *mustererrors.AppError {
	if err == nil {
		return nil
	}

	// Caller ended the call: context cancellation and deadlines are
	// timeouts (§6: connection/dial timeout, context deadline).
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return mustererrors.Wrap(err, mustererrors.CodeTimeout,
			fmt.Sprintf("asyncapi: %s: call ended by the caller's context", opContext))
	}

	// Typed kafka protocol errors: the code decides. A WriteErrors batch
	// (one error per produced message) is classified by its first typed
	// member so a per-message broker refusal still reaches the right row
	// of the §6 table.
	var wes kafka.WriteErrors
	if errors.As(err, &wes) {
		for _, we := range wes {
			if we != nil {
				return mapKafkaError(we, opContext)
			}
		}
	}
	var ke kafka.Error
	if errors.As(err, &ke) {
		appErr := mustererrors.Wrap(err, kafkaCodeToAppCode(ke), fmt.Sprintf(
			"asyncapi: %s: broker refused with %s (code %d)", opContext, ke.Error(), int(ke)))
		attachErrorDetails(appErr, err)
		return appErr
	}

	// Network timeouts (dial/read deadlines).
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return mustererrors.Wrap(err, mustererrors.CodeTimeout,
			fmt.Sprintf("asyncapi: %s: broker connection timed out", opContext))
	}

	return mustererrors.Wrap(err, mustererrors.CodeGeneric,
		fmt.Sprintf("asyncapi: %s: broker transport failure", opContext))
}

// kafkaCodeToAppCode maps kafka protocol error codes onto AppError codes
// (PROTO-007 §6). The mapping is table-driven over the typed constants.
func kafkaCodeToAppCode(ke kafka.Error) mustererrors.Code {
	switch ke {
	case kafka.ThrottlingQuotaExceeded, kafka.PolicyViolation:
		// Broker quota / throttling / publish or consume rate refusal.
		return mustererrors.CodeRateLimit
	case kafka.UnsupportedSASLMechanism, kafka.IllegalSASLState, kafka.SASLAuthenticationFailed:
		// SASL or equivalent authentication failure.
		return mustererrors.CodeUnauthorized
	case kafka.MessageSizeTooLarge, kafka.InvalidMessageSize, kafka.InvalidRecord,
		kafka.InvalidMessage, kafka.InvalidRequiredAcks, kafka.InvalidTopic:
		// Message payload/header encode or broker-side record validation
		// failure: the serialization phase failed (§6 CodeInternal row).
		return mustererrors.CodeInternal
	case kafka.RequestTimedOut:
		// The broker's own request timeout.
		return mustererrors.CodeTimeout
	default:
		// UnknownTopicOrPartition, NotLeaderForPartition, broker
		// availability and everything else: other broker/transport
		// failures.
		return mustererrors.CodeGeneric
	}
}

// kafkaErrorCode exposes the typed protocol code of a kafka error, for
// details attached to mapped errors.
func kafkaErrorCode(err error) (int, bool) {
	var ke kafka.Error
	if errors.As(err, &ke) {
		return int(ke), true
	}
	return 0, false
}

// rateLimitDetails attaches broker retry hints to a mapped rate-limit
// error when the transport exposes them (§6: preserve broker retry hints
// in details when available).
func attachErrorDetails(appErr *mustererrors.AppError, err error) {
	if appErr == nil || appErr.Code() != mustererrors.CodeRateLimit {
		return
	}
	if code, ok := kafkaErrorCode(err); ok {
		appErr.Details()["kafka_error_code"] = code
	}
}

// clock is the seam the executor reads wall time from (tests freeze it).
var clock = time.Now
