// Copyright (c) 2025 AccelByte Inc. All Rights Reserved.
// This is licensed software from AccelByte Inc, for limitations
// and restrictions contact your company contract manager.

package common

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/AccelByte/accelbyte-go-sdk/services-api/pkg/utils/auth/validator"
)

// initRetryJitterPercent spreads retries so that restarting pods do not hit IAM in lockstep.
const initRetryJitterPercent = 20

// Retry pacing. Variables rather than constants only so that tests can shrink them.
var (
	// initialInitRetryDelay is how long to wait before the second Initialize attempt.
	initialInitRetryDelay = 1 * time.Second

	// maxInitRetryDelay caps the exponential backoff between Initialize attempts.
	maxInitRetryDelay = 30 * time.Second
)

// validatorReady reports whether Validator has successfully loaded the IAM public keys. Until it
// has, Validate can only fail with "public key not found", which is a property of this server and
// not of the caller's token.
var validatorReady atomic.Bool

// IsValidatorReady reports whether the token validator finished initializing successfully.
func IsValidatorReady() bool {
	return validatorReady.Load()
}

// SetValidatorReady records whether the token validator is usable.
func SetValidatorReady(ready bool) {
	validatorReady.Store(ready)
}

// InitializeValidatorWithRetry keeps calling Initialize until it succeeds or ctx is done, marking
// the validator ready on success.
//
// A failed Initialize leaves the SDK validator holding no public keys, and on SDK versions that
// start their refresh goroutines only after a successful fetch there is nothing scheduled to refill
// them, so a single blip at startup would make the process reject every token for its whole
// lifetime. Retrying is safe on both SDK generations: a failed Initialize returns before spawning
// any goroutine, and newer SDKs guard the goroutine start with a sync.Once.
func InitializeValidatorWithRetry(ctx context.Context, tokenValidator validator.AuthTokenValidator, logger *slog.Logger) {
	delay := initialInitRetryDelay

	for attempt := 1; ; attempt++ {
		err := tokenValidator.Initialize(ctx)
		if err == nil {
			SetValidatorReady(true)
			logger.Info("token validator initialized", "attempt", attempt)

			return
		}

		// The SDK error names the stage that failed -- client login, JWKS, or revocation list --
		// which is the first thing an operator needs in order to tell a bad credential from a
		// wrong environment.
		logger.Error("token validator initialization failed",
			"attempt", attempt,
			"retryIn", delay.String(),
			"error", err.Error(),
		)

		select {
		case <-ctx.Done():
			logger.Warn("stopped retrying token validator initialization",
				"attempts", attempt,
				"reason", ctx.Err().Error(),
			)

			return
		case <-time.After(withJitter(delay)):
		}

		delay = nextRetryDelay(delay)
	}
}

// nextRetryDelay doubles the delay up to maxInitRetryDelay.
func nextRetryDelay(delay time.Duration) time.Duration {
	next := delay * 2
	if next > maxInitRetryDelay {
		return maxInitRetryDelay
	}

	return next
}

// withJitter spreads delay by up to initRetryJitterPercent in either direction. It reads the clock
// rather than math/rand because the jitter only needs to break lockstep between pods, and does not
// need to be unpredictable.
func withJitter(delay time.Duration) time.Duration {
	spread := int64(delay) * initRetryJitterPercent / 100
	if spread <= 0 {
		return delay
	}

	offset := time.Now().UnixNano()%(2*spread) - spread

	return delay + time.Duration(offset)
}
