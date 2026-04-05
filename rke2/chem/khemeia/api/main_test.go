package main

import (
	"testing"
)

func TestEmptyDirVolume(t *testing.T) {
	vol := emptyDirVolume("scratch")

	if vol.Name != "scratch" {
		t.Errorf("expected volume name %q, got %q", "scratch", vol.Name)
	}
	if vol.VolumeSource.EmptyDir == nil {
		t.Fatal("expected EmptyDir volume source, got nil")
	}
}

func TestEmptyDirMount(t *testing.T) {
	mount := emptyDirMount("scratch", "/data")

	if mount.Name != "scratch" {
		t.Errorf("expected mount name %q, got %q", "scratch", mount.Name)
	}
	if mount.MountPath != "/data" {
		t.Errorf("expected mount path %q, got %q", "/data", mount.MountPath)
	}
}

func TestPtrInt32(t *testing.T) {
	p := ptrInt32(300)

	if p == nil {
		t.Fatal("expected non-nil pointer")
	}
	if *p != 300 {
		t.Errorf("expected *p == 300, got %d", *p)
	}

	// Verify distinct pointers for distinct calls.
	p2 := ptrInt32(300)
	if p == p2 {
		t.Error("expected distinct pointers for separate calls")
	}
}

func TestAllowedWorkflowColumns(t *testing.T) {
	allowed := []string{
		"current_step", "started_at", "completed_at",
		"batch_count", "completed_batches", "message", "result", "phase",
	}
	for _, col := range allowed {
		if !allowedWorkflowColumns[col] {
			t.Errorf("expected column %q to be allowed", col)
		}
	}

	rejected := []string{
		"name", "pdbid", "source_db", "DROP TABLE", "", "receptor_pdbqt",
	}
	for _, col := range rejected {
		if allowedWorkflowColumns[col] {
			t.Errorf("expected column %q to be rejected", col)
		}
	}
}

func TestHasLogsSuffix(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/api/v1/dockingjobs/my-job/logs", true},
		{"/logs", false},            // too short (len <= 6)
		{"x/logs", false},           // exactly 6 chars, not > 6
		{"xx/logs", true},           // 7 chars, > 6 and ends with /logs
		{"/api/v1/dockingjobs/job", false},
		{"", false},
		{"/logs/extra", false},
		{"/api/v1/dockingjobs/my-job/results", false},
	}

	for _, tt := range tests {
		got := hasLogsSuffix(tt.path)
		if got != tt.want {
			t.Errorf("hasLogsSuffix(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestHasResultsSuffix(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/api/v1/dockingjobs/my-job/results", true},
		{"/results", false},         // too short (len <= 9)
		{"x/results", false},        // exactly 9 chars, not > 9
		{"xx/results", true},        // 10 chars, > 9 and ends with /results
		{"/api/v1/dockingjobs/job", false},
		{"", false},
		{"/results/extra", false},
		{"/api/v1/dockingjobs/my-job/logs", false},
	}

	for _, tt := range tests {
		got := hasResultsSuffix(tt.path)
		if got != tt.want {
			t.Errorf("hasResultsSuffix(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}
