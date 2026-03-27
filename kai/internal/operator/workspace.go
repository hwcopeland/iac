package operator

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	workspaceStorageClass = "longhorn"
	workspaceStorage      = "10Gi"
)

// workspacePVCName derives the PVC name from a run ID.
// Uses the first 8 characters of the run ID to keep names short while still
// being unique enough for sequential single-run PVCs.
func workspacePVCName(runID string) string {
	if len(runID) > 8 {
		return "kai-workspace-" + runID[:8]
	}
	return "kai-workspace-" + runID
}

// EnsureWorkspacePVC creates a PVC for the given run if it does not already
// exist. It returns the PVC name so callers can set spec.workspaceClaimName.
// The call is idempotent — calling it when the PVC already exists is safe.
func EnsureWorkspacePVC(ctx context.Context, c client.Client, namespace, runID string) (string, error) {
	name := workspacePVCName(runID)
	storageClass := workspaceStorageClass
	accessMode := corev1.ReadWriteOnce

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"kai.hwcopeland.net/run-id": runID,
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &storageClass,
			AccessModes:      []corev1.PersistentVolumeAccessMode{accessMode},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(workspaceStorage),
				},
			},
		},
	}

	if err := c.Create(ctx, pvc); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return name, nil
		}
		return "", fmt.Errorf("creating workspace PVC %s: %w", name, err)
	}

	return name, nil
}

// DeleteWorkspacePVC deletes the workspace PVC for a run.
// The call is idempotent — if the PVC does not exist the error is suppressed.
func DeleteWorkspacePVC(ctx context.Context, c client.Client, namespace, runID string) error {
	name := workspacePVCName(runID)
	pvc := &corev1.PersistentVolumeClaim{}
	key := types.NamespacedName{Namespace: namespace, Name: name}

	if err := c.Get(ctx, key, pvc); err != nil {
		return client.IgnoreNotFound(err)
	}
	if err := c.Delete(ctx, pvc); client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("deleting workspace PVC %s: %w", name, err)
	}
	return nil
}
