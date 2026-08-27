// Copyright (c) 2025 AccelByte Inc. All Rights Reserved.
// This is licensed software from AccelByte Inc, for limitations
// and restrictions contact your company contract manager.

package common

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/AccelByte/accelbyte-go-sdk/services-api/pkg/service/iam"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// fakeValidator fails the first failures calls to Initialize, then succeeds, standing in for the
// SDK validator failing to reach IAM at startup.
type fakeValidator struct {
	mu          sync.Mutex
	failures    int
	attempts    int
	validateErr error
}

func (f *fakeValidator) Initialize(_ ...context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.attempts++
	if f.attempts <= f.failures {
		return errors.New("error initializing validator: unable to login with client credentials")
	}

	return nil
}

func (f *fakeValidator) Validate(_ string, _ *iam.Permission, _ *string, _ *string) error {
	return f.validateErr
}

func (f *fakeValidator) attemptCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.attempts
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// shrinkRetryDelays keeps the tests in the millisecond range and restores the production pacing.
func shrinkRetryDelays(t *testing.T) {
	t.Helper()

	initialDelay, cappedDelay := initialInitRetryDelay, maxInitRetryDelay
	initialInitRetryDelay, maxInitRetryDelay = time.Millisecond, 4*time.Millisecond

	t.Cleanup(func() {
		initialInitRetryDelay, maxInitRetryDelay = initialDelay, cappedDelay
	})
}

// resetValidatorState isolates tests from the package level Validator and readiness flag.
func resetValidatorState(t *testing.T) {
	t.Helper()

	t.Cleanup(func() {
		Validator = nil
		SetValidatorReady(false)
	})
}

func TestInitializeValidatorWithRetryBecomesReadyAfterFailures(t *testing.T) {
	shrinkRetryDelays(t)
	resetValidatorState(t)

	fake := &fakeValidator{failures: 3}
	SetValidatorReady(false)

	InitializeValidatorWithRetry(context.Background(), fake, discardLogger())

	if !IsValidatorReady() {
		t.Fatal("validator should be ready after Initialize eventually succeeded")
	}

	if got, want := fake.attemptCount(), 4; got != want {
		t.Errorf("Initialize attempts = %d, want %d", got, want)
	}
}

func TestInitializeValidatorWithRetryStaysNotReadyWhileFailing(t *testing.T) {
	shrinkRetryDelays(t)
	resetValidatorState(t)

	fake := &fakeValidator{failures: math.MaxInt}
	SetValidatorReady(false)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		InitializeValidatorWithRetry(ctx, fake, discardLogger())
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("InitializeValidatorWithRetry did not return after its context was done")
	}

	if IsValidatorReady() {
		t.Error("validator should not be ready while Initialize keeps failing")
	}

	if fake.attemptCount() < 2 {
		t.Errorf("Initialize attempts = %d, want at least 2 retries before the context expired", fake.attemptCount())
	}
}

func TestCheckAuthorizationMetadataUnavailableWhenNotReady(t *testing.T) {
	resetValidatorState(t)

	Validator = &fakeValidator{}
	SetValidatorReady(false)

	// Metadata is present and well formed: the point is that readiness is checked first, so the
	// caller is told to retry instead of being told its token was rejected.
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer token"))

	err := checkAuthorizationMetadata(ctx)
	if got := status.Code(err); got != codes.Unavailable {
		t.Errorf("status code = %s, want %s (err = %v)", got, codes.Unavailable, err)
	}
}

func TestCheckAuthorizationMetadataWhenReady(t *testing.T) {
	resetValidatorState(t)

	withToken := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer token"))

	testCases := []struct {
		name        string
		ctx         context.Context
		validateErr error
		wantCode    codes.Code
	}{
		{
			name:     "valid token is accepted",
			ctx:      withToken,
			wantCode: codes.OK,
		},
		{
			name:        "rejected token stays permission denied",
			ctx:         withToken,
			validateErr: errors.New("token was revoked"),
			wantCode:    codes.PermissionDenied,
		},
		{
			name:     "missing metadata stays unauthenticated",
			ctx:      context.Background(),
			wantCode: codes.Unauthenticated,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			Validator = &fakeValidator{validateErr: testCase.validateErr}
			SetValidatorReady(true)

			err := checkAuthorizationMetadata(testCase.ctx)
			if got := status.Code(err); got != testCase.wantCode {
				t.Errorf("status code = %s, want %s (err = %v)", got, testCase.wantCode, err)
			}
		})
	}
}

func TestRetryDelayIsBounded(t *testing.T) {
	delay := initialInitRetryDelay
	for i := 0; i < 20; i++ {
		delay = nextRetryDelay(delay)
		if delay > maxInitRetryDelay {
			t.Fatalf("delay grew to %s past the %s cap after %d doublings", delay, maxInitRetryDelay, i+1)
		}
	}

	if delay != maxInitRetryDelay {
		t.Errorf("delay settled at %s, want the %s cap", delay, maxInitRetryDelay)
	}

	lowest := maxInitRetryDelay - maxInitRetryDelay*initRetryJitterPercent/100
	highest := maxInitRetryDelay + maxInitRetryDelay*initRetryJitterPercent/100
	for i := 0; i < 1000; i++ {
		jittered := withJitter(maxInitRetryDelay)
		if jittered < lowest || jittered > highest {
			t.Fatalf("withJitter(%s) = %s, want within [%s, %s]", maxInitRetryDelay, jittered, lowest, highest)
		}
	}
}
