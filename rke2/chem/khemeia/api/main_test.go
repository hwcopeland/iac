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
