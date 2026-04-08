// Package main provides the generic K8s job runner for the plugin system.
// RunPluginJob replaces the old processQEJob and processDockingJob functions
// with a single implementation that works for any plugin type.
package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RunPluginJob executes a compute job defined by a plugin specification.
// For type "job", it:
//  1. Updates status to Running
//  2. Creates a ConfigMap with input data (text fields become files, others become env vars)
//  3. Creates a K8s Job with the plugin's image, command, and resources
//  4. Waits for completion with timeout and orphan pod detection
//  5. Reads pod logs and parses output using the plugin's output regex patterns
//  6. Stores parsed output as JSON in the output_data column
//  7. Sets status to Completed or Failed
func (c *Controller) RunPluginJob(plugin Plugin, jobName string, input map[string]interface{}) {
	log.Printf("[%s] Starting %s job with plugin %s", jobName, plugin.Type, plugin.Name)

	db := c.pluginDB(plugin.Slug)
	if db == nil {
		log.Printf("[%s] CRITICAL: no database for plugin %s", jobName, plugin.Slug)
		return
	}

	// 1. Update status to Running.
	if _, err := db.Exec(
		fmt.Sprintf(`UPDATE %s SET status='Running', started_at=NOW() WHERE name=?`, plugin.TableName()),
		jobName); err != nil {
		log.Printf("[%s] Failed to update status to Running: %v", jobName, err)
		return
	}

	ctx := context.Background()
	cmName := fmt.Sprintf("%s-input-%s", plugin.Slug, jobName)

	// 2. Create ConfigMap with input data.
	cmData := make(map[string]string)
	cmBinary := make(map[string][]byte)

	for _, field := range plugin.Input {
		val, exists := input[field.Name]
		if !exists || val == nil {
			continue
		}

		if field.Type == "text" {
			// Text fields become files in the ConfigMap.
			s, ok := val.(string)
			if ok {
				cmData["input.in"] = s

				// For QE compatibility: look up pseudopotentials from DB.
				if plugin.Slug == "qe" {
					loadPseudopotentials(db, jobName, s, cmBinary)
				}
			}
		}
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: cmName,
			Labels: map[string]string{
				"app":                 fmt.Sprintf("%s-job", plugin.Slug),
				"khemeia.io/plugin":   plugin.Slug,
				"khemeia.io/job-name": jobName,
			},
		},
		Data:       cmData,
		BinaryData: cmBinary,
	}
	if _, err := c.client.CoreV1().ConfigMaps(c.namespace).Create(ctx, cm, metav1.CreateOptions{}); err != nil {
		failPluginJob(db, plugin, jobName, fmt.Sprintf("failed to create input ConfigMap: %v", err))
		return
	}

	// Cleanup ConfigMap when done.
	defer func() {
		if err := c.client.CoreV1().ConfigMaps(c.namespace).Delete(ctx, cmName, metav1.DeleteOptions{}); err != nil {
			log.Printf("[%s] Warning: failed to delete ConfigMap %s: %v", jobName, cmName, err)
		}
	}()

	// 3. Expand command template with input values.
	expandedCommand := plugin.ExpandCommand(input)

	// Expand resource templates.
	cpuStr := plugin.ExpandResource(plugin.Resources.CPU, input)
	memStr := plugin.ExpandResource(plugin.Resources.Memory, input)

	// 4. Create the K8s Job.
	backoffLimit := int32(0)
	j := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: jobName,
			Labels: map[string]string{
				"app":                 fmt.Sprintf("%s-job", plugin.Slug),
				"khemeia.io/plugin":   plugin.Slug,
				"khemeia.io/job-name": jobName,
			},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: ptrInt32(600),
			BackoffLimit:            &backoffLimit,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:    corev1.RestartPolicyNever,
					ImagePullSecrets: []corev1.LocalObjectReference{{Name: "zot-pull-secret"}},
					Containers: []corev1.Container{
						{
							Name:            plugin.Slug,
							Image:           plugin.Image,
							ImagePullPolicy: corev1.PullIfNotPresent,
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
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "input",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{Name: cmName},
								},
							},
						},
						emptyDirVolume("scratch"),
					},
				},
			},
		},
	}

	if _, err := c.jobClient.Create(ctx, j, metav1.CreateOptions{}); err != nil {
		failPluginJob(db, plugin, jobName, fmt.Sprintf("failed to create K8s Job: %v", err))
		return
	}

	// 5. Stream logs for observability.
	streamCtx, streamCancel := context.WithCancel(ctx)
	go c.streamJobLogs(streamCtx, jobName)

	// 6. Wait for completion with plugin-appropriate timeout.
	if err := c.waitForPluginJobCompletion(jobName, plugin.TimeoutDuration()); err != nil {
		streamCancel()
		failPluginJob(db, plugin, jobName, fmt.Sprintf("job execution failed: %v", err))
		return
	}
	streamCancel()

	// 7. Capture output from pod logs.
	output, err := c.readPodLogs(jobName)
	if err != nil {
		failPluginJob(db, plugin, jobName, fmt.Sprintf("failed to read pod logs: %v", err))
		return
	}

	// 7a. Extract base64-encoded artifacts from stdout markers and store them.
	artifacts := extractArtifacts(output)
	if len(artifacts) > 0 {
		for filename, data := range artifacts {
			contentType := guessContentType(filename)
			_, err := db.ExecContext(ctx,
				`INSERT INTO job_artifacts (job_name, filename, content_type, size_bytes, content)
				 VALUES (?, ?, ?, ?, ?) ON DUPLICATE KEY UPDATE content = VALUES(content), size_bytes = VALUES(size_bytes)`,
				jobName, filename, contentType, len(data), data)
			if err != nil {
				log.Printf("[%s] Warning: failed to store artifact %s: %v", jobName, filename, err)
			}
		}
		log.Printf("[%s] Stored %d artifacts", jobName, len(artifacts))
	}

	// 7b. Strip artifact blocks from stdout before regex parsing so base64 data
	// doesn't confuse the output parsers.
	output = stripArtifactBlocks(output)

	// 8. Parse output using plugin's output regex patterns.
	parsedOutput := plugin.ParseOutput(output)

	// For text output fields without a parse regex, store the full log output.
	for _, outField := range plugin.Output {
		if outField.Type == "text" && outField.Parse == "" {
			parsedOutput[outField.Name] = output
		}
	}

	// 9. Store output as JSON.
	outputJSON, err := json.Marshal(parsedOutput)
	if err != nil {
		log.Printf("[%s] Warning: failed to marshal output: %v", jobName, err)
		outputJSON = []byte("{}")
	}

	if _, err := db.Exec(
		fmt.Sprintf(`UPDATE %s SET status='Completed', output_data=?, completed_at=NOW() WHERE name=?`, plugin.TableName()),
		string(outputJSON), jobName); err != nil {
		log.Printf("[%s] CRITICAL: failed to store results: %v", jobName, err)
		return
	}

	log.Printf("[%s] Job completed successfully", jobName)
}

// waitForPluginJobCompletion polls for job completion with the given timeout.
// Includes orphan pod detection to handle lost kubelet completion events.
func (c *Controller) waitForPluginJobCompletion(jobName string, timeout time.Duration) error {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	timeoutCh := time.After(timeout)
	pollCount := 0

	for {
		select {
		case <-timeoutCh:
			return fmt.Errorf("timeout after %v waiting for job %s", timeout, jobName)
		case <-ticker.C:
			pollCount++
			job, err := c.jobClient.Get(context.TODO(), jobName, metav1.GetOptions{})
			if err != nil {
				if errors.IsNotFound(err) {
					if pollCount <= 3 || pollCount%30 == 0 {
						log.Printf("[wait] %s: not found yet (poll %d)", jobName, pollCount)
					}
					continue
				}
				return err
			}
			if pollCount%30 == 0 {
				log.Printf("[wait] %s: active=%d succeeded=%d failed=%d (poll %d)",
					jobName, job.Status.Active, job.Status.Succeeded, job.Status.Failed, pollCount)
			}
			if job.Status.Succeeded > 0 {
				log.Printf("[wait] %s: succeeded after %d polls", jobName, pollCount)
				return nil
			}
			if job.Status.Failed > 0 {
				return fmt.Errorf("job %s failed", jobName)
			}

			// Orphan pod detection: if the Job says active but no pods exist,
			// the kubelet lost the completion event.
			if job.Status.Active > 0 && pollCount > 3 {
				pods, perr := c.client.CoreV1().Pods(c.namespace).List(context.TODO(), metav1.ListOptions{
					LabelSelector: fmt.Sprintf("job-name=%s", jobName),
				})
				if perr == nil && len(pods.Items) == 0 {
					log.Printf("[wait] %s: Job says active but no pods exist -- treating as succeeded (poll %d)", jobName, pollCount)
					return nil
				}
			}
		}
	}
}

// failPluginJob marks a plugin job as Failed and stores the error output.
func failPluginJob(db *sql.DB, plugin Plugin, jobName, message string) {
	log.Printf("[%s] Job failed: %s", jobName, message)
	if _, err := db.Exec(
		fmt.Sprintf(`UPDATE %s SET status='Failed', error_output=?, completed_at=NOW() WHERE name=?`, plugin.TableName()),
		message, jobName); err != nil {
		log.Printf("[%s] CRITICAL: failed to mark job as Failed: %v", jobName, err)
	}
}

// buildJobEnv creates environment variables for the job container.
// Passes through MySQL connection info and all non-text input fields as uppercase env vars.
func buildJobEnv(plugin Plugin, jobName string, input map[string]interface{}) []corev1.EnvVar {
	envs := []corev1.EnvVar{
		{Name: "JOB_NAME", Value: jobName},
		{Name: "WORKFLOW_NAME", Value: jobName},
		{Name: "MYSQL_HOST", Value: os.Getenv("MYSQL_HOST")},
		{Name: "MYSQL_PORT", Value: os.Getenv("MYSQL_PORT")},
		{Name: "MYSQL_USER", Value: os.Getenv("MYSQL_USER")},
		{Name: "MYSQL_PASSWORD", Value: os.Getenv("MYSQL_PASSWORD")},
		{Name: "MYSQL_DATABASE", Value: plugin.Database},
	}

	// Pass input fields as env vars (uppercase names).
	for _, field := range plugin.Input {
		val, exists := input[field.Name]
		if !exists || val == nil {
			continue
		}
		if field.Type == "text" {
			continue // text fields are mounted as files, not env vars
		}
		envName := strings.ToUpper(field.Name)
		envs = append(envs, corev1.EnvVar{
			Name:  envName,
			Value: fmt.Sprintf("%v", val),
		})
	}

	return envs
}

// loadPseudopotentials finds .UPF references in a QE input file and loads them
// from the database into the ConfigMap's BinaryData.
func loadPseudopotentials(db *sql.DB, jobName, inputFile string, cmBinary map[string][]byte) {
	for _, line := range strings.Split(inputFile, "\n") {
		line = strings.TrimSpace(line)
		fields := strings.Fields(line)
		for _, f := range fields {
			if strings.HasSuffix(strings.ToUpper(f), ".UPF") {
				var content []byte
				err := db.QueryRow(`SELECT content FROM pseudopotentials WHERE filename = ?`, f).Scan(&content)
				if err == nil && len(content) > 0 {
					cmBinary[f] = content
					log.Printf("[%s] Loaded pseudopotential %s from DB (%d bytes)", jobName, f, len(content))
				} else {
					log.Printf("[%s] Pseudopotential %s not in DB, will try download at runtime", jobName, f)
				}
			}
		}
	}
}

// extractArtifacts scans log output for base64-encoded artifact blocks delimited
// by marker lines. Plugin commands emit artifacts between:
//
//	===ARTIFACT:<filename>===
//	<base64-encoded content>
//	===END_ARTIFACT===
//
// Returns a map of filename to decoded binary content. Malformed or undecodable
// blocks are logged and skipped.
func extractArtifacts(logOutput string) map[string][]byte {
	artifacts := make(map[string][]byte)

	const startPrefix = "===ARTIFACT:"
	const startSuffix = "==="
	const endMarker = "===END_ARTIFACT==="

	lines := strings.Split(logOutput, "\n")
	i := 0
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])

		// Look for ===ARTIFACT:<filename>===
		if !strings.HasPrefix(line, startPrefix) {
			i++
			continue
		}

		// Extract filename from the marker line.
		rest := line[len(startPrefix):]
		if !strings.HasSuffix(rest, startSuffix) {
			i++
			continue
		}
		filename := rest[:len(rest)-len(startSuffix)]
		if filename == "" {
			i++
			continue
		}

		// Collect base64 content lines until the end marker.
		i++
		var b64Lines []string
		found := false
		for i < len(lines) {
			trimmed := strings.TrimSpace(lines[i])
			if trimmed == endMarker {
				found = true
				i++
				break
			}
			b64Lines = append(b64Lines, trimmed)
			i++
		}

		if !found {
			log.Printf("Warning: artifact %q missing end marker, skipping", filename)
			continue
		}

		// Decode the base64 content.
		b64Content := strings.Join(b64Lines, "")
		decoded, err := base64.StdEncoding.DecodeString(b64Content)
		if err != nil {
			log.Printf("Warning: artifact %q has invalid base64: %v, skipping", filename, err)
			continue
		}

		artifacts[filename] = decoded
	}

	return artifacts
}

// stripArtifactBlocks removes artifact marker blocks (===ARTIFACT:...=== through
// ===END_ARTIFACT===) from log output so the base64 data does not interfere with
// the plugin's output regex parsers.
func stripArtifactBlocks(output string) string {
	const startPrefix = "===ARTIFACT:"
	const endMarker = "===END_ARTIFACT==="

	lines := strings.Split(output, "\n")
	var result []string
	i := 0
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, startPrefix) {
			// Skip lines until we find the end marker or run out of input.
			i++
			for i < len(lines) {
				if strings.TrimSpace(lines[i]) == endMarker {
					i++
					break
				}
				i++
			}
			continue
		}
		result = append(result, lines[i])
		i++
	}

	return strings.Join(result, "\n")
}

// guessContentType maps file extensions to MIME content types for artifact storage.
func guessContentType(filename string) string {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".cube":
		return "chemical/x-cube"
	case ".molden":
		return "chemical/x-molden"
	case ".pdbqt":
		return "chemical/x-pdbqt"
	case ".json":
		return "application/json"
	case ".dat":
		return "text/plain"
	case ".hess":
		return "application/octet-stream"
	default:
		return "application/octet-stream"
	}
}
