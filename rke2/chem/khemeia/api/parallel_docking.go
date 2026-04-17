// Package main provides parallel docking job orchestration.
// When a docking job is submitted, the controller:
//  1. Runs a single leader pod for protein prep + workflow registration
//  2. Launches N parallel prep pods to convert SMILES → PDBQT
//  3. Launches N parallel docking pods, each docking its chunk
//  4. Waits for all workers, then marks the job as Completed
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"sync"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RunParallelDockingJob orchestrates a docking job across multiple worker pods.
func (c *Controller) RunParallelDockingJob(plugin Plugin, jobName string, input map[string]interface{}) {
	log.Printf("[%s] Starting parallel docking job", jobName)

	db := c.pluginDB(plugin.Slug)
	if db == nil {
		log.Printf("[%s] CRITICAL: no database for plugin %s", jobName, plugin.Slug)
		return
	}

	if _, err := db.Exec(
		fmt.Sprintf(`UPDATE %s SET status='Running', started_at=NOW() WHERE name=?`, plugin.TableName()),
		jobName); err != nil {
		log.Printf("[%s] Failed to update status to Running: %v", jobName, err)
		return
	}

	pdbid, _ := input["pdbid"].(string)
	ligandDB, _ := input["ligand_db"].(string)
	nativeLigand, _ := input["native_ligand"].(string)
	if nativeLigand == "" {
		nativeLigand = "TTT"
	}
	chunkSize := 10000
	if cs, ok := input["chunk_size"].(float64); ok && cs > 0 {
		chunkSize = int(cs)
	}

	if pdbid == "" || ligandDB == "" {
		failPluginJob(db, plugin, jobName, "pdbid and ligand_db are required")
		return
	}

	cpuStr := plugin.ExpandResource(plugin.Resources.CPU, input)
	memStr := plugin.ExpandResource(plugin.Resources.Memory, input)

	// ── Phase 1: Protein prep (single pod) — skip if receptor already exists ──
	var existingReceptor int
	_ = db.QueryRow(
		`SELECT LENGTH(receptor_pdbqt) FROM docking_workflows WHERE pdbid = ? AND receptor_pdbqt IS NOT NULL LIMIT 1`,
		pdbid,
	).Scan(&existingReceptor)

	if existingReceptor > 0 {
		log.Printf("[%s] Phase 1: Receptor for %s already prepped (%d bytes), skipping protein prep", jobName, pdbid, existingReceptor)
		// Copy workflow record for this job name
		_, _ = db.Exec(
			`INSERT INTO docking_workflows (name, pdbid, source_db, native_ligand, chunk_size, image, receptor_pdbqt, grid_center_x, grid_center_y, grid_center_z, phase)
			 SELECT ?, pdbid, ?, ?, ?, 'generic', receptor_pdbqt, grid_center_x, grid_center_y, grid_center_z, 'Running'
			 FROM docking_workflows WHERE pdbid = ? AND receptor_pdbqt IS NOT NULL LIMIT 1
			 ON DUPLICATE KEY UPDATE receptor_pdbqt=VALUES(receptor_pdbqt)`,
			jobName, ligandDB, nativeLigand, chunkSize, pdbid,
		)
	} else {
		log.Printf("[%s] Phase 1: Protein prep", jobName)
		leaderName := fmt.Sprintf("%s-prep", jobName)
		if err := c.runProteinPrepPod(plugin, leaderName, jobName, input, cpuStr, memStr); err != nil {
			failPluginJob(db, plugin, jobName, fmt.Sprintf("protein prep failed: %v", err))
			return
		}
	}

	// ── Phase 2: Parallel ligand prep ───────────────────────────────
	var unprepCount int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM ligands WHERE source_db = ? AND pdbqt IS NULL`, ligandDB,
	).Scan(&unprepCount); err != nil {
		failPluginJob(db, plugin, jobName, fmt.Sprintf("failed to count unprepped ligands: %v", err))
		return
	}

	if unprepCount > 0 {
		prepWorkers := int(math.Ceil(float64(unprepCount) / float64(chunkSize)))
		log.Printf("[%s] Phase 2: Launching %d prep workers for %d unprepped ligands", jobName, prepWorkers, unprepCount)

		if err := c.runParallelWorkers(jobName, prepWorkers, chunkSize, unprepCount, "prep",
			func(workerName string, offset, limit int) error {
				return c.createPrepWorker(plugin, workerName, jobName, ligandDB, offset, limit, cpuStr, memStr)
			}, 2*time.Hour); err != nil {
			failPluginJob(db, plugin, jobName, fmt.Sprintf("ligand prep failed: %v", err))
			return
		}
		log.Printf("[%s] Phase 2 complete: ligand prep finished", jobName)
	} else {
		log.Printf("[%s] Phase 2: All ligands already prepped, skipping", jobName)
	}

	// ── Phase 3: Parallel docking ───────────────────────────────────
	var totalLigands int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM ligands WHERE source_db = ? AND pdbqt IS NOT NULL`, ligandDB,
	).Scan(&totalLigands); err != nil || totalLigands == 0 {
		failPluginJob(db, plugin, jobName, "no prepped ligands found")
		return
	}

	dockWorkers := int(math.Ceil(float64(totalLigands) / float64(chunkSize)))
	log.Printf("[%s] Phase 3: Launching %d docking workers for %d ligands", jobName, dockWorkers, totalLigands)

	if err := c.runParallelWorkers(jobName, dockWorkers, chunkSize, totalLigands, "dock",
		func(workerName string, offset, limit int) error {
			return c.createDockingWorker(plugin, workerName, jobName, input, offset, limit, cpuStr, memStr)
		}, plugin.TimeoutDuration()); err != nil {
		failPluginJob(db, plugin, jobName, fmt.Sprintf("docking failed: %v", err))
		return
	}

	// ── Finalize ────────────────────────────────────────────────────
	var resultCount int
	_ = db.QueryRow(
		`SELECT COUNT(*) FROM docking_results WHERE workflow_name = ?`, jobName,
	).Scan(&resultCount)

	var bestAffinity sql.NullFloat64
	_ = db.QueryRow(
		`SELECT MIN(affinity_kcal_mol) FROM docking_results WHERE workflow_name = ?`, jobName,
	).Scan(&bestAffinity)

	outputJSON, _ := json.Marshal(map[string]interface{}{
		"result_count":   resultCount,
		"best_affinity":  bestAffinity.Float64,
		"dock_workers":   dockWorkers,
		"total_ligands":  totalLigands,
	})

	if _, err := db.Exec(
		fmt.Sprintf(`UPDATE %s SET status='Completed', output_data=?, completed_at=NOW() WHERE name=?`, plugin.TableName()),
		string(outputJSON), jobName); err != nil {
		log.Printf("[%s] CRITICAL: failed to store results: %v", jobName, err)
		return
	}

	log.Printf("[%s] Docking complete: %d results from %d workers, best=%.1f kcal/mol",
		jobName, resultCount, dockWorkers, bestAffinity.Float64)
}

// runParallelWorkers launches N worker pods and waits for all to complete.
// createFn is called for each worker with (workerName, offset, limit).
func (c *Controller) runParallelWorkers(
	jobName string,
	workerCount, chunkSize, totalItems int,
	role string,
	createFn func(workerName string, offset, limit int) error,
	timeout time.Duration,
) error {
	workerNames := make([]string, workerCount)
	for i := 0; i < workerCount; i++ {
		offset := i * chunkSize
		limit := chunkSize
		if offset+limit > totalItems {
			limit = totalItems - offset
		}

		workerName := fmt.Sprintf("%s-%s-%d", jobName, role, i)
		workerNames[i] = workerName

		if err := createFn(workerName, offset, limit); err != nil {
			return fmt.Errorf("failed to create %s worker %d: %v", role, i, err)
		}
		log.Printf("[%s] Launched %s worker %s (offset=%d, limit=%d)", jobName, role, workerName, offset, limit)
	}

	log.Printf("[%s] Waiting for %d %s workers...", jobName, workerCount, role)
	var wg sync.WaitGroup
	workerErrors := make([]error, workerCount)

	for i, wn := range workerNames {
		wg.Add(1)
		go func(idx int, name string) {
			defer wg.Done()
			if err := c.waitForPluginJobCompletion(name, timeout); err != nil {
				workerErrors[idx] = err
				log.Printf("[%s] %s worker %s failed: %v", jobName, role, name, err)
			} else {
				log.Printf("[%s] %s worker %s completed", jobName, role, name)
			}
		}(i, wn)
	}

	wg.Wait()

	failed := 0
	for _, err := range workerErrors {
		if err != nil {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d/%d %s workers failed", failed, workerCount, role)
	}
	return nil
}

// runProteinPrepPod runs protein prep + workflow registration in a single pod.
// This is the only phase that uses the plugin command template (which does
// proteinprepv2.py + registers the workflow in docking_workflows).
func (c *Controller) runProteinPrepPod(plugin Plugin, leaderName, jobName string, input map[string]interface{}, cpuStr, memStr string) error {
	ctx := context.Background()
	expandedCommand := plugin.ExpandCommand(input)

	cmName := fmt.Sprintf("docking-input-%s", jobName)
	cmData := make(map[string]string)
	for _, field := range plugin.Input {
		val, exists := input[field.Name]
		if !exists || val == nil || field.Type != "text" {
			continue
		}
		if s, ok := val.(string); ok {
			cmData["input.in"] = s
		}
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:   cmName,
			Labels: map[string]string{"khemeia.io/job-name": jobName},
		},
		Data: cmData,
	}
	if _, err := c.client.CoreV1().ConfigMaps(c.namespace).Create(ctx, cm, metav1.CreateOptions{}); err != nil {
		log.Printf("[%s] ConfigMap create: %v (may already exist)", leaderName, err)
	}

	backoffLimit := int32(0)
	j := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:   leaderName,
			Labels: map[string]string{"khemeia.io/job-name": jobName, "khemeia.io/role": "protein-prep"},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: ptrInt32(600),
			BackoffLimit:            &backoffLimit,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:    corev1.RestartPolicyNever,
					ImagePullSecrets: []corev1.LocalObjectReference{{Name: "zot-pull-secret"}},
					Containers: []corev1.Container{{
						Name:            "prep",
						Image:           plugin.Image,
						ImagePullPolicy: corev1.PullAlways,
						Command:         []string{"/bin/sh", "-c"},
						Args:            []string{expandedCommand},
						Env:             buildJobEnv(plugin, jobName, input),
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse(cpuStr),
								corev1.ResourceMemory: resource.MustParse(memStr),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse(cpuStr),
								corev1.ResourceMemory: resource.MustParse(memStr),
							},
						},
						VolumeMounts: []corev1.VolumeMount{
							{Name: "input", MountPath: "/input", ReadOnly: true},
							emptyDirMount("scratch", "/scratch"),
							emptyDirMount("data", "/data"),
						},
					}},
					Volumes: []corev1.Volume{
						{Name: "input", VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{Name: cmName},
							},
						}},
						emptyDirVolume("scratch"),
						emptyDirVolume("data"),
					},
				},
			},
		},
	}

	if _, err := c.jobClient.Create(ctx, j, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("failed to create leader pod: %v", err)
	}

	streamCtx, streamCancel := context.WithCancel(ctx)
	go c.streamJobLogs(streamCtx, leaderName)
	defer streamCancel()

	return c.waitForPluginJobCompletion(leaderName, 2*time.Hour)
}

// createPrepWorker creates a K8s Job that runs prep_ligands.py for a chunk.
func (c *Controller) createPrepWorker(plugin Plugin, workerName, jobName, sourceDB string, offset, limit int, cpuStr, memStr string) error {
	backoffLimit := int32(0)
	j := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: workerName,
			Labels: map[string]string{
				"khemeia.io/job-name": jobName,
				"khemeia.io/role":     "prep-worker",
			},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: ptrInt32(600),
			BackoffLimit:            &backoffLimit,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:    corev1.RestartPolicyNever,
					ImagePullSecrets: []corev1.LocalObjectReference{{Name: "zot-pull-secret"}},
					Containers: []corev1.Container{{
						Name:            "prep",
						Image:           plugin.Image,
						ImagePullPolicy: corev1.PullAlways,
						Command:         []string{"python3", "/autodock/scripts/prep_ligands.py"},
						Env: []corev1.EnvVar{
							{Name: "SOURCE_DB", Value: sourceDB},
							{Name: "BATCH_OFFSET", Value: fmt.Sprintf("%d", offset)},
							{Name: "BATCH_LIMIT", Value: fmt.Sprintf("%d", limit)},
							{Name: "MYSQL_HOST", Value: os.Getenv("MYSQL_HOST")},
							{Name: "MYSQL_PORT", Value: os.Getenv("MYSQL_PORT")},
							{Name: "MYSQL_USER", Value: os.Getenv("MYSQL_USER")},
							{Name: "MYSQL_PASSWORD", Value: os.Getenv("MYSQL_PASSWORD")},
							{Name: "MYSQL_DATABASE", Value: plugin.Database},
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("1"),
								corev1.ResourceMemory: resource.MustParse("512Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("1"),
								corev1.ResourceMemory: resource.MustParse("512Mi"),
							},
						},
						VolumeMounts: []corev1.VolumeMount{emptyDirMount("data", "/data")},
					}},
					Volumes: []corev1.Volume{emptyDirVolume("data")},
				},
			},
		},
	}

	_, err := c.jobClient.Create(context.TODO(), j, metav1.CreateOptions{})
	return err
}

// createDockingWorker creates a K8s Job that runs dock_batch.py for a chunk.
func (c *Controller) createDockingWorker(plugin Plugin, workerName, jobName string, input map[string]interface{}, offset, limit int, cpuStr, memStr string) error {
	pdbid, _ := input["pdbid"].(string)
	nativeLigand, _ := input["native_ligand"].(string)
	if nativeLigand == "" {
		nativeLigand = "TTT"
	}
	ligandDB, _ := input["ligand_db"].(string)

	backoffLimit := int32(0)
	j := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: workerName,
			Labels: map[string]string{
				"khemeia.io/job-name": jobName,
				"khemeia.io/role":     "dock-worker",
			},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: ptrInt32(600),
			BackoffLimit:            &backoffLimit,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:    corev1.RestartPolicyNever,
					ImagePullSecrets: []corev1.LocalObjectReference{{Name: "zot-pull-secret"}},
					Containers: []corev1.Container{{
						Name:            "dock",
						Image:           plugin.Image,
						ImagePullPolicy: corev1.PullAlways,
						Command:         []string{"python3", "/autodock/scripts/dock_batch.py"},
						Env: []corev1.EnvVar{
							{Name: "WORKFLOW_NAME", Value: jobName},
							{Name: "JOB_NAME", Value: jobName},
							{Name: "PDBID", Value: pdbid},
							{Name: "NATIVE_LIGAND", Value: nativeLigand},
							{Name: "SOURCE_DB", Value: ligandDB},
							{Name: "BATCH_OFFSET", Value: fmt.Sprintf("%d", offset)},
							{Name: "BATCH_LIMIT", Value: fmt.Sprintf("%d", limit)},
							{Name: "NUM_CPUS", Value: cpuStr},
							{Name: "MYSQL_HOST", Value: os.Getenv("MYSQL_HOST")},
							{Name: "MYSQL_PORT", Value: os.Getenv("MYSQL_PORT")},
							{Name: "MYSQL_USER", Value: os.Getenv("MYSQL_USER")},
							{Name: "MYSQL_PASSWORD", Value: os.Getenv("MYSQL_PASSWORD")},
							{Name: "MYSQL_DATABASE", Value: plugin.Database},
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse(cpuStr),
								corev1.ResourceMemory: resource.MustParse(memStr),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse(cpuStr),
								corev1.ResourceMemory: resource.MustParse(memStr),
							},
						},
						VolumeMounts: []corev1.VolumeMount{emptyDirMount("data", "/data")},
					}},
					Volumes: []corev1.Volume{emptyDirVolume("data")},
				},
			},
		},
	}

	_, err := c.jobClient.Create(context.TODO(), j, metav1.CreateOptions{})
	return err
}
