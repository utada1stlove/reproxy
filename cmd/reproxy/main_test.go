package main

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"
)

type stubStartupSyncer struct {
	err   error
	calls int
}

func (s *stubStartupSyncer) Sync(context.Context) error {
	s.calls++
	return s.err
}

func TestSyncOnStartupLogsAndContinuesOnFailure(t *testing.T) {
	var logBuffer bytes.Buffer
	logger := log.New(&logBuffer, "", 0)
	syncer := &stubStartupSyncer{err: errors.New("certificate hook failed")}

	syncOnStartup(context.Background(), logger, syncer)

	if syncer.calls != 1 {
		t.Fatalf("expected sync to be called once, got %d", syncer.calls)
	}

	output := logBuffer.String()
	if !strings.Contains(output, "continuing in degraded mode") {
		t.Fatalf("expected degraded-mode log, got %q", output)
	}
}

func TestSyncOnStartupDoesNotLogOnSuccess(t *testing.T) {
	var logBuffer bytes.Buffer
	logger := log.New(&logBuffer, "", 0)
	syncer := &stubStartupSyncer{}

	syncOnStartup(context.Background(), logger, syncer)

	if syncer.calls != 1 {
		t.Fatalf("expected sync to be called once, got %d", syncer.calls)
	}

	if logBuffer.Len() != 0 {
		t.Fatalf("expected no startup warning log, got %q", logBuffer.String())
	}
}
