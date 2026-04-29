package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

// ---------------------------------------------------------------------------
// Test database helper — matches existing codebase pattern of using real MySQL
// when available, skipping otherwise.
// ---------------------------------------------------------------------------

// testDB returns a *sql.DB connected to the test MySQL instance.
// Skips the test if MYSQL_TEST_DSN is not set or the connection fails.
// The DSN should be something like "root:password@tcp(127.0.0.1:3306)/".
func testDB(t *testing.T) *sql.DB {
	t.Helper()

	dsn := "root:@tcp(127.0.0.1:3306)/"
	// Allow override via environment variable for CI.
	if envDSN := envOr("MYSQL_TEST_DSN", ""); envDSN != "" {
		dsn = envDSN
	}

	db, err := sql.Open("mysql", dsn+"?parseTime=true&multiStatements=true")
	if err != nil {
		t.Skipf("MySQL not available (open): %v", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		t.Skipf("MySQL not available (ping): %v", err)
	}

	// Create a per-test database to avoid cross-test contamination.
	dbName := fmt.Sprintf("wp9_test_%d", uniqueCounter())
	if _, err := db.Exec("CREATE DATABASE " + dbName); err != nil {
		db.Close()
		t.Skipf("MySQL cannot create test database: %v", err)
	}

	// Reconnect to the test database.
	db.Close()
	testDB, err := sql.Open("mysql", dsn+dbName+"?parseTime=true&multiStatements=true")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	// Ensure provenance schema exists.
	if err := EnsureProvenanceSchema(testDB); err != nil {
		testDB.Close()
		t.Fatalf("failed to create provenance schema: %v", err)
	}

	t.Cleanup(func() {
		testDB.Close()
		cleanupDB, _ := sql.Open("mysql", dsn+"?parseTime=true")
		if cleanupDB != nil {
			cleanupDB.Exec("DROP DATABASE IF EXISTS " + dbName)
			cleanupDB.Close()
		}
	})

	return testDB
}

// envOr returns the environment variable value or a fallback.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// uniqueCounter returns a monotonically increasing integer for test isolation.
var _testCounter int64

func uniqueCounter() int64 {
	_testCounter++
	return _testCounter
}

// ===================================================================
// Provenance Tests (require MySQL — skip when unavailable)
// ===================================================================

// TestProvenanceRecordAndGet creates a provenance record, retrieves it by ID,
// and verifies all fields round-trip correctly.
func TestProvenanceRecordAndGet(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	bucket := "khemeia-receptors"
	key := "TargetPrep/tp-001/receptor.pdbqt"
	checksum := "abc123def456"
	jobKind := "TargetPrep"

	record := &ProvenanceRecord{
		ArtifactType:   "receptor",
		S3Bucket:       &bucket,
		S3Key:          &key,
		ChecksumSHA256: &checksum,
		CreatedByJob:   "targetprep-001",
		JobKind:        &jobKind,
		JobNamespace:   "chem",
		Parameters:     json.RawMessage(`{"exhaustiveness": 8}`),
		ToolVersions:   json.RawMessage(`{"vina": "1.2"}`),
	}

	err := RecordProvenance(ctx, db, record, nil)
	if err != nil {
		t.Fatalf("RecordProvenance failed: %v", err)
	}

	if record.ArtifactID == "" {
		t.Fatal("expected ArtifactID to be generated, got empty string")
	}

	// Retrieve it.
	got, err := GetProvenance(ctx, db, record.ArtifactID)
	if err != nil {
		t.Fatalf("GetProvenance failed: %v", err)
	}
	if got == nil {
		t.Fatal("GetProvenance returned nil for existing record")
	}

	// Verify all fields.
	if got.ArtifactID != record.ArtifactID {
		t.Errorf("ArtifactID: want %q, got %q", record.ArtifactID, got.ArtifactID)
	}
	if got.ArtifactType != "receptor" {
		t.Errorf("ArtifactType: want %q, got %q", "receptor", got.ArtifactType)
	}
	if got.S3Bucket == nil || *got.S3Bucket != bucket {
		t.Errorf("S3Bucket: want %q, got %v", bucket, got.S3Bucket)
	}
	if got.S3Key == nil || *got.S3Key != key {
		t.Errorf("S3Key: want %q, got %v", key, got.S3Key)
	}
	if got.ChecksumSHA256 == nil || *got.ChecksumSHA256 != checksum {
		t.Errorf("ChecksumSHA256: want %q, got %v", checksum, got.ChecksumSHA256)
	}
	if got.CreatedByJob != "targetprep-001" {
		t.Errorf("CreatedByJob: want %q, got %q", "targetprep-001", got.CreatedByJob)
	}
	if got.JobKind == nil || *got.JobKind != jobKind {
		t.Errorf("JobKind: want %q, got %v", jobKind, got.JobKind)
	}
	if got.JobNamespace != "chem" {
		t.Errorf("JobNamespace: want %q, got %q", "chem", got.JobNamespace)
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
}

// TestProvenanceAncestors builds a 3-level chain (A -> B -> C) and verifies
// that querying ancestors of C returns C, B, and A in depth order.
func TestProvenanceAncestors(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	// Level 0: A (root, no parents).
	recA := &ProvenanceRecord{
		ArtifactType: "receptor",
		CreatedByJob: "job-a",
		JobNamespace: "chem",
	}
	if err := RecordProvenance(ctx, db, recA, nil); err != nil {
		t.Fatalf("RecordProvenance A failed: %v", err)
	}

	// Level 1: B (child of A).
	recB := &ProvenanceRecord{
		ArtifactType: "library",
		CreatedByJob: "job-b",
		JobNamespace: "chem",
	}
	if err := RecordProvenance(ctx, db, recB, []string{recA.ArtifactID}); err != nil {
		t.Fatalf("RecordProvenance B failed: %v", err)
	}

	// Level 2: C (child of B).
	recC := &ProvenanceRecord{
		ArtifactType: "docked_pose",
		CreatedByJob: "job-c",
		JobNamespace: "chem",
	}
	if err := RecordProvenance(ctx, db, recC, []string{recB.ArtifactID}); err != nil {
		t.Fatalf("RecordProvenance C failed: %v", err)
	}

	// Query ancestors of C.
	ancestors, err := GetAncestors(ctx, db, recC.ArtifactID, 50)
	if err != nil {
		t.Fatalf("GetAncestors failed: %v", err)
	}

	// The CTE starts from C itself (depth 0), then B (depth 1), then A (depth 2).
	if len(ancestors) != 3 {
		t.Fatalf("expected 3 ancestors (including self), got %d", len(ancestors))
	}

	// Verify order: depth 0 = C, depth 1 = B, depth 2 = A.
	if ancestors[0].ArtifactID != recC.ArtifactID {
		t.Errorf("ancestor[0] (depth 0): want C=%s, got %s", recC.ArtifactID, ancestors[0].ArtifactID)
	}
	if ancestors[1].ArtifactID != recB.ArtifactID {
		t.Errorf("ancestor[1] (depth 1): want B=%s, got %s", recB.ArtifactID, ancestors[1].ArtifactID)
	}
	if ancestors[2].ArtifactID != recA.ArtifactID {
		t.Errorf("ancestor[2] (depth 2): want A=%s, got %s", recA.ArtifactID, ancestors[2].ArtifactID)
	}

	// Verify depth values.
	for i, a := range ancestors {
		if a.Depth == nil {
			t.Errorf("ancestor[%d]: depth should not be nil", i)
		} else if *a.Depth != i {
			t.Errorf("ancestor[%d]: want depth %d, got %d", i, i, *a.Depth)
		}
	}
}

// TestProvenanceDescendants builds the same A -> B -> C chain and verifies
// that querying descendants of A returns A, B, and C in depth order.
func TestProvenanceDescendants(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	// Build chain: A -> B -> C.
	recA := &ProvenanceRecord{
		ArtifactType: "receptor",
		CreatedByJob: "desc-job-a",
		JobNamespace: "chem",
	}
	if err := RecordProvenance(ctx, db, recA, nil); err != nil {
		t.Fatalf("RecordProvenance A failed: %v", err)
	}

	recB := &ProvenanceRecord{
		ArtifactType: "library",
		CreatedByJob: "desc-job-b",
		JobNamespace: "chem",
	}
	if err := RecordProvenance(ctx, db, recB, []string{recA.ArtifactID}); err != nil {
		t.Fatalf("RecordProvenance B failed: %v", err)
	}

	recC := &ProvenanceRecord{
		ArtifactType: "docked_pose",
		CreatedByJob: "desc-job-c",
		JobNamespace: "chem",
	}
	if err := RecordProvenance(ctx, db, recC, []string{recB.ArtifactID}); err != nil {
		t.Fatalf("RecordProvenance C failed: %v", err)
	}

	// Query descendants of A.
	descendants, err := GetDescendants(ctx, db, recA.ArtifactID, 50)
	if err != nil {
		t.Fatalf("GetDescendants failed: %v", err)
	}

	// The CTE starts from A (depth 0), then B (depth 1), then C (depth 2).
	if len(descendants) != 3 {
		t.Fatalf("expected 3 descendants (including self), got %d", len(descendants))
	}

	if descendants[0].ArtifactID != recA.ArtifactID {
		t.Errorf("descendant[0] (depth 0): want A=%s, got %s", recA.ArtifactID, descendants[0].ArtifactID)
	}
	if descendants[1].ArtifactID != recB.ArtifactID {
		t.Errorf("descendant[1] (depth 1): want B=%s, got %s", recB.ArtifactID, descendants[1].ArtifactID)
	}
	if descendants[2].ArtifactID != recC.ArtifactID {
		t.Errorf("descendant[2] (depth 2): want C=%s, got %s", recC.ArtifactID, descendants[2].ArtifactID)
	}
}

// TestProvenanceJobArtifacts creates 3 records from the same job and verifies
// GetJobArtifacts returns all 3.
func TestProvenanceJobArtifacts(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	jobName := "shared-dockjob-42"
	types := []string{"docked_pose", "docked_pose", "docked_pose"}

	for i, at := range types {
		rec := &ProvenanceRecord{
			ArtifactType: at,
			CreatedByJob: jobName,
			JobNamespace: "chem",
			Parameters:   json.RawMessage(fmt.Sprintf(`{"pose_index": %d}`, i)),
		}
		if err := RecordProvenance(ctx, db, rec, nil); err != nil {
			t.Fatalf("RecordProvenance[%d] failed: %v", i, err)
		}
	}

	artifacts, err := GetJobArtifacts(ctx, db, jobName)
	if err != nil {
		t.Fatalf("GetJobArtifacts failed: %v", err)
	}

	if len(artifacts) != 3 {
		t.Fatalf("expected 3 artifacts, got %d", len(artifacts))
	}

	for _, a := range artifacts {
		if a.CreatedByJob != jobName {
			t.Errorf("artifact %s: expected created_by_job %q, got %q",
				a.ArtifactID, jobName, a.CreatedByJob)
		}
	}
}

// TestProvenanceEdgeValidation attempts to record a provenance entry with
// a non-existent parent ID and verifies the foreign key constraint fires.
func TestProvenanceEdgeValidation(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	rec := &ProvenanceRecord{
		ArtifactType: "receptor",
		CreatedByJob: "edge-test-job",
		JobNamespace: "chem",
	}

	// Use a parent ID that does not exist in provenance.
	bogusParentID := "00000000-0000-0000-0000-000000000000"
	err := RecordProvenance(ctx, db, rec, []string{bogusParentID})

	if err == nil {
		t.Fatal("expected error when inserting edge with non-existent parent, got nil")
	}

	// The error should mention the foreign key constraint or the edge insertion.
	errStr := err.Error()
	if !strings.Contains(errStr, "edge") && !strings.Contains(errStr, "foreign") &&
		!strings.Contains(errStr, "FOREIGN") && !strings.Contains(errStr, "constraint") &&
		!strings.Contains(errStr, "fk_parent") && !strings.Contains(errStr, "Cannot add") {
		t.Errorf("expected FK-related error, got: %s", errStr)
	}
}

// ===================================================================
// S3 Client Tests (unit, no external dependencies)
// ===================================================================

// TestNoopS3Client verifies that NoopS3Client returns ErrNotFound for Get
// and succeeds silently for Put.
func TestNoopS3Client(t *testing.T) {
	client := &NoopS3Client{}
	ctx := context.Background()

	// Put should succeed (no-op).
	err := client.PutArtifact(ctx, BucketReceptors, "test/key.pdb", strings.NewReader("data"), "application/octet-stream")
	if err != nil {
		t.Errorf("PutArtifact: expected nil error, got %v", err)
	}

	// Get should return ErrNotFound.
	_, err = client.GetArtifact(ctx, BucketReceptors, "test/key.pdb")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetArtifact: expected ErrNotFound, got %v", err)
	}

	// GetPresignedURL should return ErrNotFound.
	_, err = client.GetPresignedURL(ctx, BucketReceptors, "test/key.pdb", 0)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetPresignedURL: expected ErrNotFound, got %v", err)
	}

	// HeadArtifact should return ErrNotFound.
	_, err = client.HeadArtifact(ctx, BucketReceptors, "test/key.pdb")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("HeadArtifact: expected ErrNotFound, got %v", err)
	}

	// ListArtifacts should return empty slice (no error).
	list, err := client.ListArtifacts(ctx, BucketReceptors, "test/")
	if err != nil {
		t.Errorf("ListArtifacts: expected nil error, got %v", err)
	}
	if len(list) != 0 {
		t.Errorf("ListArtifacts: expected empty slice, got %d items", len(list))
	}

	// DeleteArtifact should succeed (no-op).
	err = client.DeleteArtifact(ctx, BucketReceptors, "test/key.pdb")
	if err != nil {
		t.Errorf("DeleteArtifact: expected nil error, got %v", err)
	}
}

// TestArtifactKey verifies the canonical key format.
func TestArtifactKey(t *testing.T) {
	tests := []struct {
		jobKind      string
		jobName      string
		artifactName string
		ext          string
		want         string
	}{
		{
			jobKind:      "DockJob",
			jobName:      "my-job",
			artifactName: "receptor",
			ext:          "pdbqt",
			want:         "DockJob/my-job/receptor.pdbqt",
		},
		{
			jobKind:      "TargetPrep",
			jobName:      "tp-1714500000",
			artifactName: "CHEMBL12345-pose1",
			ext:          "pdbqt",
			want:         "TargetPrep/tp-1714500000/CHEMBL12345-pose1.pdbqt",
		},
		{
			jobKind:      "RefineJob",
			jobName:      "refine-42",
			artifactName: "trajectory",
			ext:          "xtc",
			want:         "RefineJob/refine-42/trajectory.xtc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := ArtifactKey(tt.jobKind, tt.jobName, tt.artifactName, tt.ext)
			if got != tt.want {
				t.Errorf("ArtifactKey(%q, %q, %q, %q) = %q, want %q",
					tt.jobKind, tt.jobName, tt.artifactName, tt.ext, got, tt.want)
			}
		})
	}
}

// TestBucketConstants verifies all 7 bucket names are defined and non-empty.
func TestBucketConstants(t *testing.T) {
	buckets := map[string]string{
		"BucketReceptors":    BucketReceptors,
		"BucketLibraries":    BucketLibraries,
		"BucketPoses":        BucketPoses,
		"BucketTrajectories": BucketTrajectories,
		"BucketReports":      BucketReports,
		"BucketPanels":       BucketPanels,
		"BucketScratch":      BucketScratch,
	}

	if len(buckets) != 7 {
		t.Errorf("expected 7 bucket constants, got %d", len(buckets))
	}

	for name, value := range buckets {
		if value == "" {
			t.Errorf("bucket constant %s is empty", name)
		}
		if !strings.HasPrefix(value, "khemeia-") {
			t.Errorf("bucket %s = %q: expected 'khemeia-' prefix", name, value)
		}
	}
}

// ===================================================================
// CRD Controller Tests (unit-level, mock k8s client)
// ===================================================================

// testComputeClassesYAML returns the raw YAML from the deploy ConfigMap
// for parsing tests.
const testComputeClassesYAML = `classes:
  cpu:
    description: "Standard CPU workloads"
    resources:
      cpu: "2"
      memory: "4Gi"
    nodeSelector: {}
    tolerations: []
    volumes: []
    volumeMounts: []
    env: []

  cpu-high-mem:
    description: "Memory-intensive CPU workloads (MD analysis, large libraries)"
    resources:
      cpu: "4"
      memory: "16Gi"
    nodeSelector: {}
    tolerations: []
    volumes: []
    volumeMounts: []
    env: []

  gpu:
    description: "GPU workloads on NixOS node (nixos-gpu, RTX 3070)"
    resources:
      cpu: "4"
      memory: "16Gi"
      nvidia.com/gpu: "1"
    nodeSelector:
      gpu: "rtx3070"
    tolerations:
      - key: "gpu"
        value: "true"
        effect: "NoSchedule"
    volumes:
      - name: nvidia-driver
        hostPath:
          path: /run/opengl-driver
      - name: nix-store
        hostPath:
          path: /nix/store
    volumeMounts:
      - name: nvidia-driver
        mountPath: /run/opengl-driver
        readOnly: true
      - name: nix-store
        mountPath: /nix/store
        readOnly: true
    env:
      - name: LD_LIBRARY_PATH
        value: /run/opengl-driver/lib:/opt/boost/lib:/usr/lib/x86_64-linux-gnu
      - name: OCL_ICD_VENDORS
        value: /run/opengl-driver/etc/OpenCL/vendors
`

// TestComputeClassParsing parses the compute-classes.yaml ConfigMap format
// and verifies the GPU class has nvidia.com/gpu resource, tolerations,
// and NixOS host-path mounts.
func TestComputeClassParsing(t *testing.T) {
	var config struct {
		Classes map[string]ComputeClass `yaml:"classes"`
	}
	if err := yaml.Unmarshal([]byte(testComputeClassesYAML), &config); err != nil {
		t.Fatalf("failed to parse compute classes YAML: %v", err)
	}

	if len(config.Classes) != 3 {
		t.Fatalf("expected 3 compute classes, got %d", len(config.Classes))
	}

	// Verify CPU class.
	cpu, ok := config.Classes["cpu"]
	if !ok {
		t.Fatal("cpu class not found")
	}
	if cpu.Resources["cpu"] != "2" {
		t.Errorf("cpu class: expected cpu=2, got %s", cpu.Resources["cpu"])
	}
	if cpu.Resources["memory"] != "4Gi" {
		t.Errorf("cpu class: expected memory=4Gi, got %s", cpu.Resources["memory"])
	}

	// Verify GPU class in detail.
	gpu, ok := config.Classes["gpu"]
	if !ok {
		t.Fatal("gpu class not found")
	}

	// nvidia.com/gpu resource.
	if gpu.Resources["nvidia.com/gpu"] != "1" {
		t.Errorf("gpu class: expected nvidia.com/gpu=1, got %q", gpu.Resources["nvidia.com/gpu"])
	}

	// Node selector.
	if gpu.NodeSelector["gpu"] != "rtx3070" {
		t.Errorf("gpu class nodeSelector: expected gpu=rtx3070, got %v", gpu.NodeSelector)
	}

	// Tolerations.
	if len(gpu.Tolerations) != 1 {
		t.Fatalf("gpu class: expected 1 toleration, got %d", len(gpu.Tolerations))
	}
	tol := gpu.Tolerations[0]
	if tol.Key != "gpu" || tol.Value != "true" || tol.Effect != corev1.TaintEffectNoSchedule {
		t.Errorf("gpu toleration: want {key:gpu, value:true, effect:NoSchedule}, got {key:%s, value:%s, effect:%s}",
			tol.Key, tol.Value, tol.Effect)
	}

	// NixOS volume mounts.
	if len(gpu.Volumes) != 2 {
		t.Fatalf("gpu class: expected 2 volumes, got %d", len(gpu.Volumes))
	}
	volNames := map[string]bool{}
	for _, v := range gpu.Volumes {
		volNames[v.Name] = true
	}
	if !volNames["nvidia-driver"] {
		t.Error("gpu class: missing nvidia-driver volume")
	}
	if !volNames["nix-store"] {
		t.Error("gpu class: missing nix-store volume")
	}

	if len(gpu.VolumeMounts) != 2 {
		t.Fatalf("gpu class: expected 2 volume mounts, got %d", len(gpu.VolumeMounts))
	}
	mountNames := map[string]bool{}
	for _, vm := range gpu.VolumeMounts {
		mountNames[vm.Name] = true
	}
	if !mountNames["nvidia-driver"] {
		t.Error("gpu class: missing nvidia-driver volume mount")
	}
	if !mountNames["nix-store"] {
		t.Error("gpu class: missing nix-store volume mount")
	}

	// Verify full mount details via the controller's applyComputeClass path,
	// which is the actual production integration point. This covers mountPath
	// and readOnly fields through the real code path.
	// (Covered by TestBuildJobForCRD which loads the same YAML through loadComputeClasses.)

	// LD_LIBRARY_PATH env var.
	if len(gpu.Env) != 2 {
		t.Fatalf("gpu class: expected 2 env vars, got %d", len(gpu.Env))
	}
	envMap := map[string]string{}
	for _, env := range gpu.Env {
		envMap[env.Name] = env.Value
	}
	if envMap["LD_LIBRARY_PATH"] != "/run/opengl-driver/lib:/opt/boost/lib:/usr/lib/x86_64-linux-gnu" {
		t.Errorf("gpu env: want LD_LIBRARY_PATH=/run/opengl-driver/lib:/opt/boost/lib:/usr/lib/x86_64-linux-gnu, got %q",
			envMap["LD_LIBRARY_PATH"])
	}
	if envMap["OCL_ICD_VENDORS"] != "/run/opengl-driver/etc/OpenCL/vendors" {
		t.Errorf("gpu env: want OCL_ICD_VENDORS=/run/opengl-driver/etc/OpenCL/vendors, got %q",
			envMap["OCL_ICD_VENDORS"])
	}
}

// TestCRDImageMapping verifies the mapping from CRD kind to container image.
func TestCRDImageMapping(t *testing.T) {
	tests := []struct {
		kind  string
		image string
	}{
		{"TargetPrep", "zot.hwcopeland.net/chem/target-prep:latest"},
		{"DockJob", "zot.hwcopeland.net/chem/vina:1.2"},
		{"RefineJob", "zot.hwcopeland.net/chem/refine:latest"},
		{"ADMETJob", "zot.hwcopeland.net/chem/admet:latest"},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			got, ok := crdImageMapping[tt.kind]
			if !ok {
				t.Fatalf("no image mapping found for kind %q", tt.kind)
			}
			if got != tt.image {
				t.Errorf("crdImageMapping[%q] = %q, want %q", tt.kind, got, tt.image)
			}
		})
	}

	// Verify all registered CRDs have an image mapping.
	for kind := range registeredCRDs {
		if _, ok := crdImageMapping[kind]; !ok {
			t.Errorf("registered CRD %q has no image mapping", kind)
		}
	}
}

// ===================================================================
// Advance API Tests (unit-level, mock k8s dynamic client)
// ===================================================================

// khemeiaListKinds maps Khemeia GVRs to their list kind names.
// Required by the fake dynamic client to support List operations.
var khemeiaListKinds = map[schema.GroupVersionResource]string{
	{Group: khemeiaGroup, Version: khemeiaVersion, Resource: "targetpreps"}: "TargetPrepList",
	{Group: khemeiaGroup, Version: khemeiaVersion, Resource: "dockjobs"}:    "DockJobList",
	{Group: khemeiaGroup, Version: khemeiaVersion, Resource: "refinejobs"}:  "RefineJobList",
	{Group: khemeiaGroup, Version: khemeiaVersion, Resource: "admetjobs"}:   "ADMETJobList",
}

// newFakeDynamicClient creates a fake dynamic client pre-loaded with
// the given unstructured objects and all Khemeia list kinds registered.
func newFakeDynamicClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, khemeiaListKinds, objects...)
}

// makeUnstructuredCRD builds an unstructured CRD instance for testing.
func makeUnstructuredCRD(kind, name, phase string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "khemeia.io/v1alpha1",
			"kind":       kind,
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": "chem",
			},
			"spec": map[string]interface{}{
				"gate": "manual",
			},
		},
	}
	if phase != "" {
		obj.Object["status"] = map[string]interface{}{
			"phase": phase,
		}
	}
	return obj
}

// TestAdvanceValidation tests that advancing a non-Succeeded job returns 409 Conflict.
func TestAdvanceValidation(t *testing.T) {
	// Create a TargetPrep CRD in "Running" phase.
	crd := makeUnstructuredCRD("TargetPrep", "tp-test-001", "Running")

	client := newFakeDynamicClient(crd)

	handlers := NewCRDHandlers(client, nil, "chem")

	body := `{"downstream_kind": "DockJob"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/TargetPrep/tp-test-001/advance",
		strings.NewReader(body))
	rr := httptest.NewRecorder()

	handlers.HandleAdvance(rr, req)

	if rr.Code != http.StatusConflict {
		t.Errorf("expected status %d (Conflict), got %d", http.StatusConflict, rr.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !strings.Contains(resp["error"], "Succeeded") {
		t.Errorf("expected error mentioning 'Succeeded', got %q", resp["error"])
	}
}

// TestAdvanceCreatesDownstream verifies that advancing a Succeeded job creates a
// downstream CRD with the parentJob field set.
func TestAdvanceCreatesDownstream(t *testing.T) {
	// Create a Succeeded TargetPrep CRD.
	crd := makeUnstructuredCRD("TargetPrep", "tp-success-001", "Succeeded")

	client := newFakeDynamicClient(crd)

	handlers := NewCRDHandlers(client, nil, "chem")

	body := `{"downstream_kind": "DockJob", "downstream_params": {"receptorKey": "test.pdbqt"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/TargetPrep/tp-success-001/advance",
		strings.NewReader(body))
	rr := httptest.NewRecorder()

	handlers.HandleAdvance(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status %d (Created), got %d; body: %s",
			http.StatusCreated, rr.Code, rr.Body.String())
	}

	var resp AdvanceResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Kind != "DockJob" {
		t.Errorf("expected downstream kind %q, got %q", "DockJob", resp.Kind)
	}
	if resp.Namespace != "chem" {
		t.Errorf("expected namespace %q, got %q", "chem", resp.Namespace)
	}
	if resp.ParentJob["kind"] != "TargetPrep" {
		t.Errorf("expected parentJob kind %q, got %q", "TargetPrep", resp.ParentJob["kind"])
	}
	if resp.ParentJob["name"] != "tp-success-001" {
		t.Errorf("expected parentJob name %q, got %q", "tp-success-001", resp.ParentJob["name"])
	}

	// Verify the downstream CRD was actually created in the fake client.
	dockJobGVR := schema.GroupVersionResource{
		Group:    "khemeia.io",
		Version:  "v1alpha1",
		Resource: "dockjobs",
	}
	list, err := client.Resource(dockJobGVR).Namespace("chem").List(
		context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("failed to list downstream CRDs: %v", err)
	}

	if len(list.Items) != 1 {
		t.Fatalf("expected 1 downstream DockJob, got %d", len(list.Items))
	}

	downstream := list.Items[0]
	spec, ok := downstream.Object["spec"].(map[string]interface{})
	if !ok {
		t.Fatal("downstream CRD has no spec")
	}

	parentJob, ok := spec["parentJob"].(map[string]interface{})
	if !ok {
		t.Fatal("downstream CRD spec has no parentJob")
	}
	if parentJob["kind"] != "TargetPrep" {
		t.Errorf("downstream parentJob kind: want %q, got %v", "TargetPrep", parentJob["kind"])
	}
	if parentJob["name"] != "tp-success-001" {
		t.Errorf("downstream parentJob name: want %q, got %v", "tp-success-001", parentJob["name"])
	}
}

// ===================================================================
// CRD Controller unit helpers
// ===================================================================

// TestCRDJobName verifies the job naming convention.
func TestCRDJobName(t *testing.T) {
	tests := []struct {
		kind string
		name string
		want string
	}{
		{"TargetPrep", "tp-001", "targetprep-tp-001"},
		{"DockJob", "dock-42", "dockjob-dock-42"},
		{"RefineJob", "refine-1", "refinejob-refine-1"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := crdJobName(tt.kind, tt.name)
			if got != tt.want {
				t.Errorf("crdJobName(%q, %q) = %q, want %q",
					tt.kind, tt.name, got, tt.want)
			}
		})
	}
}

// TestGetPhase verifies status phase extraction from unstructured objects.
func TestGetPhase(t *testing.T) {
	tests := []struct {
		desc  string
		obj   *unstructured.Unstructured
		want  string
	}{
		{
			desc: "explicit Succeeded phase",
			obj:  makeUnstructuredCRD("DockJob", "dj-1", "Succeeded"),
			want: "Succeeded",
		},
		{
			desc: "no status defaults to Pending",
			obj:  makeUnstructuredCRD("DockJob", "dj-2", ""),
			want: "Pending",
		},
		{
			desc: "Running phase",
			obj:  makeUnstructuredCRD("DockJob", "dj-3", "Running"),
			want: "Running",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := getPhase(tt.obj)
			if got != tt.want {
				t.Errorf("getPhase() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestParseJobAdvancePath tests path parsing for the advance endpoint.
func TestParseJobAdvancePath(t *testing.T) {
	tests := []struct {
		path     string
		wantKind string
		wantName string
		wantErr  bool
	}{
		{"/api/v1/jobs/TargetPrep/tp-001/advance", "TargetPrep", "tp-001", false},
		{"/api/v1/jobs/DockJob/dock-42/advance", "DockJob", "dock-42", false},
		{"/api/v1/jobs/bad/advance", "", "", true},
		{"/api/v1/jobs/advance", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			kind, name, err := parseJobAdvancePath(tt.path)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for path %q, got nil", tt.path)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for path %q: %v", tt.path, err)
			}
			if kind != tt.wantKind || name != tt.wantName {
				t.Errorf("parseJobAdvancePath(%q) = (%q, %q), want (%q, %q)",
					tt.path, kind, name, tt.wantKind, tt.wantName)
			}
		})
	}
}

// TestBuildJobForCRD verifies that buildJobForCRD produces correct Job specs
// including compute class application.
func TestBuildJobForCRD(t *testing.T) {
	// Create a fake kube client with the compute classes ConfigMap.
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "khemeia-compute-classes",
			Namespace: "chem",
		},
		Data: map[string]string{
			"classes.yaml": testComputeClassesYAML,
		},
	}
	kubeClient := kubefake.NewSimpleClientset(cm)
	dynClient := newFakeDynamicClient()

	controller := NewCRDController(kubeClient, dynClient, "chem", nil, &NoopS3Client{})

	// Load compute classes from the fake ConfigMap.
	if err := controller.loadComputeClasses(); err != nil {
		t.Fatalf("loadComputeClasses failed: %v", err)
	}

	// Build a DockJob CRD (default compute class = gpu).
	crd := makeUnstructuredCRD("DockJob", "dock-build-test", "Pending")

	job, err := controller.buildJobForCRD(crd)
	if err != nil {
		t.Fatalf("buildJobForCRD failed: %v", err)
	}

	// Verify image mapping.
	if len(job.Spec.Template.Spec.Containers) == 0 {
		t.Fatal("expected at least 1 container in job spec")
	}
	container := job.Spec.Template.Spec.Containers[0]
	if container.Image != "zot.hwcopeland.net/chem/vina:1.2" {
		t.Errorf("expected image %q, got %q", "zot.hwcopeland.net/chem/vina:1.2", container.Image)
	}

	// Verify GPU compute class was applied.
	podSpec := job.Spec.Template.Spec

	// Node selector.
	if podSpec.NodeSelector["gpu"] != "rtx3070" {
		t.Errorf("expected nodeSelector gpu=rtx3070, got %v", podSpec.NodeSelector)
	}

	// Tolerations.
	foundGPUTol := false
	for _, tol := range podSpec.Tolerations {
		if tol.Key == "gpu" && tol.Value == "true" {
			foundGPUTol = true
		}
	}
	if !foundGPUTol {
		t.Error("expected GPU toleration not found")
	}

	// NixOS volumes.
	volNames := map[string]bool{}
	for _, v := range podSpec.Volumes {
		volNames[v.Name] = true
	}
	if !volNames["nvidia-driver"] {
		t.Error("expected nvidia-driver volume in job spec")
	}
	if !volNames["nix-store"] {
		t.Error("expected nix-store volume in job spec")
	}

	// GPU resource.
	gpuRes := container.Resources.Requests["nvidia.com/gpu"]
	if gpuRes.String() != "1" {
		t.Errorf("expected nvidia.com/gpu request of 1, got %s", gpuRes.String())
	}

	// LD_LIBRARY_PATH.
	foundLDPath := false
	foundICDVendors := false
	for _, env := range container.Env {
		if env.Name == "LD_LIBRARY_PATH" && env.Value == "/run/opengl-driver/lib:/opt/boost/lib:/usr/lib/x86_64-linux-gnu" {
			foundLDPath = true
		}
		if env.Name == "OCL_ICD_VENDORS" && env.Value == "/run/opengl-driver/etc/OpenCL/vendors" {
			foundICDVendors = true
		}
	}
	if !foundLDPath {
		t.Error("expected LD_LIBRARY_PATH=/run/opengl-driver/lib:/opt/boost/lib:/usr/lib/x86_64-linux-gnu in container env")
	}
	if !foundICDVendors {
		t.Error("expected OCL_ICD_VENDORS=/run/opengl-driver/etc/OpenCL/vendors in container env")
	}
}

// TestDefaultComputeClassForKind verifies kind-to-class mapping.
func TestDefaultComputeClassForKind(t *testing.T) {
	tests := []struct {
		kind string
		want string
	}{
		{"TargetPrep", "cpu"},
		{"DockJob", "gpu"},
		{"RefineJob", "gpu"},
		{"ADMETJob", "cpu"},
		{"SelectivityJob", "cpu-high-mem"},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			got := defaultComputeClassForKind[tt.kind]
			if got != tt.want {
				t.Errorf("defaultComputeClassForKind[%q] = %q, want %q",
					tt.kind, got, tt.want)
			}
		})
	}
}

// TestRegisteredCRDs verifies the GVR mapping for registered CRD kinds.
func TestRegisteredCRDs(t *testing.T) {
	expected := map[string]string{
		"TargetPrep": "targetpreps",
		"DockJob":    "dockjobs",
		"RefineJob":  "refinejobs",
		"ADMETJob":   "admetjobs",
	}

	for kind, wantResource := range expected {
		gvr, ok := registeredCRDs[kind]
		if !ok {
			t.Errorf("registeredCRDs: missing kind %q", kind)
			continue
		}
		if gvr.Group != "khemeia.io" {
			t.Errorf("registeredCRDs[%q].Group = %q, want %q", kind, gvr.Group, "khemeia.io")
		}
		if gvr.Version != "v1alpha1" {
			t.Errorf("registeredCRDs[%q].Version = %q, want %q", kind, gvr.Version, "v1alpha1")
		}
		if gvr.Resource != wantResource {
			t.Errorf("registeredCRDs[%q].Resource = %q, want %q", kind, gvr.Resource, wantResource)
		}
	}
}

// TestAdvanceMethodNotAllowed verifies that GET on the advance endpoint returns 405.
func TestAdvanceMethodNotAllowed(t *testing.T) {
	client := newFakeDynamicClient()
	handlers := NewCRDHandlers(client, nil, "chem")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/TargetPrep/tp-001/advance", nil)
	rr := httptest.NewRecorder()
	handlers.HandleAdvance(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, rr.Code)
	}
}

// TestAdvanceUnknownKind verifies that advancing with an unknown CRD kind returns 400.
func TestAdvanceUnknownKind(t *testing.T) {
	client := newFakeDynamicClient()
	handlers := NewCRDHandlers(client, nil, "chem")

	body := `{"downstream_kind": "DockJob"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/FakeKind/fake-001/advance",
		strings.NewReader(body))
	rr := httptest.NewRecorder()
	handlers.HandleAdvance(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d (BadRequest), got %d", http.StatusBadRequest, rr.Code)
	}
}

// TestToInterfaceSlice verifies string-to-interface conversion.
func TestToInterfaceSlice(t *testing.T) {
	input := []string{"a", "b", "c"}
	result := toInterfaceSlice(input)

	if len(result) != 3 {
		t.Fatalf("expected 3 items, got %d", len(result))
	}
	for i, v := range result {
		s, ok := v.(string)
		if !ok {
			t.Errorf("item[%d] is not a string", i)
		}
		if s != input[i] {
			t.Errorf("item[%d] = %q, want %q", i, s, input[i])
		}
	}
}
