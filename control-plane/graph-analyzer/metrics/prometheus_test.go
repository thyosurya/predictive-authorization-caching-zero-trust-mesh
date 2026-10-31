package metrics

import (
	"testing"
)

// TestNewCollector memverifikasi Collector bisa dibuat
func TestNewCollector(t *testing.T) {
	c := NewCollector()
	if c == nil {
		t.Fatal("NewCollector() returned nil")
	}
}

// TestRecordCall memverifikasi RecordCall tidak panic
func TestRecordCall(t *testing.T) {
	c := NewCollector()
	c.RecordCall("service-a", "service-b")
	c.RecordCall("service-a", "service-b")
	// Jika tidak panic, test berhasil
}

// TestSetEdgeCount memverifikasi SetEdgeCount tidak panic
func TestSetEdgeCount(t *testing.T) {
	c := NewCollector()
	c.SetEdgeCount(10)
	c.SetEdgeCount(0)
}

// TestObserveAnalysisDuration memverifikasi ObserveAnalysisDuration tidak panic
func TestObserveAnalysisDuration(t *testing.T) {
	c := NewCollector()
	c.ObserveAnalysisDuration(42.5)
	c.ObserveAnalysisDuration(0.0)
}
