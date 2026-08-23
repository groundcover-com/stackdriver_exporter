// Copyright 2026 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package collectors

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"google.golang.org/api/googleapi"
)

func TestRetryOnTransient(t *testing.T) {
	origBase, origMax := retryBaseBackoff, retryMaxBackoff
	retryBaseBackoff, retryMaxBackoff = time.Millisecond, 5*time.Millisecond
	defer func() { retryBaseBackoff, retryMaxBackoff = origBase, origMax }()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	err503 := &googleapi.Error{Code: 503}

	t.Run("transient then success", func(t *testing.T) {
		calls := 0
		err := retryOnTransient(context.Background(), logger, func() error {
			calls++
			if calls <= 2 {
				return err503
			}
			return nil
		})
		if err != nil {
			t.Errorf("expected success, got %v", err)
		}
		if calls != 3 {
			t.Errorf("expected 3 calls, got %d", calls)
		}
	})

	t.Run("persistent transient error", func(t *testing.T) {
		calls := 0
		err := retryOnTransient(context.Background(), logger, func() error {
			calls++
			return err503
		})
		if !errors.Is(err, err503) {
			t.Errorf("expected the 503 error, got %v", err)
		}
		if calls != retryMaxAttempts {
			t.Errorf("expected %d calls, got %d", retryMaxAttempts, calls)
		}
	})

	t.Run("non-retryable API error", func(t *testing.T) {
		calls := 0
		err400 := &googleapi.Error{Code: 400}
		err := retryOnTransient(context.Background(), logger, func() error {
			calls++
			return err400
		})
		if !errors.Is(err, err400) {
			t.Errorf("expected the 400 error, got %v", err)
		}
		if calls != 1 {
			t.Errorf("expected 1 call, got %d", calls)
		}
	})

	t.Run("non-googleapi error", func(t *testing.T) {
		calls := 0
		plainErr := errors.New("boom")
		err := retryOnTransient(context.Background(), logger, func() error {
			calls++
			return plainErr
		})
		if !errors.Is(err, plainErr) {
			t.Errorf("expected the plain error, got %v", err)
		}
		if calls != 1 {
			t.Errorf("expected 1 call, got %d", calls)
		}
	})

	t.Run("context canceled during backoff", func(t *testing.T) {
		retryBaseBackoff, retryMaxBackoff = time.Hour, time.Hour
		defer func() { retryBaseBackoff, retryMaxBackoff = time.Millisecond, 5*time.Millisecond }()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		calls := 0
		start := time.Now()
		err := retryOnTransient(ctx, logger, func() error {
			calls++
			return err503
		})
		if !errors.Is(err, err503) {
			t.Errorf("expected the 503 error, got %v", err)
		}
		if calls != 1 {
			t.Errorf("expected 1 call, got %d", calls)
		}
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Errorf("expected prompt return, took %v", elapsed)
		}
	})
}
