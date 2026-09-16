package main

import (
	"context"
	"errors"
	"testing"
)

// TestReportUpgradeFailureDefersToCancellation pins the contract that an
// interrupted upgrade is reported as the cancellation itself rather than a
// failed upgrade with a retry hint. The guard runs before any ui output, so
// the test stays quiet.
func TestReportUpgradeFailureDefersToCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := reportUpgradeFailure(ctx, errTestBoom)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("reportUpgradeFailure returned %v, want context.Canceled", err)
	}
}
