// Package main provides a thin S3 client wrapper for Garage (S3-compatible object store).
// The S3Client interface and GarageClient implementation are used to store and retrieve
// binary artifacts (receptors, poses, trajectories, etc.) outside of MySQL.
//
// A feature flag (GARAGE_ENABLED) controls whether operations actually hit Garage or
// degrade to no-op warnings, allowing the code to be deployed before Garage is running.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// Sentinel errors for S3 operations.
var (
	// ErrNotFound indicates the requested object does not exist.
	ErrNotFound = errors.New("s3: object not found")

	// ErrBucketNotFound indicates the target bucket does not exist.
	ErrBucketNotFound = errors.New("s3: bucket not found")
)

// Bucket constants for artifact storage. Each bucket corresponds to a logical
// artifact category in the Khemeia platform.
const (
	BucketReceptors    = "khemeia-receptors"
	BucketLibraries    = "khemeia-libraries"
	BucketPoses        = "khemeia-poses"
	BucketTrajectories = "khemeia-trajectories"
	BucketReports      = "khemeia-reports"
	BucketPanels       = "khemeia-panels"
	BucketScratch      = "khemeia-scratch"
)

// ArtifactInfo represents S3 object metadata.
type ArtifactInfo struct {
	Key          string    `json:"key"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"last_modified"`
	ContentType  string    `json:"content_type"`
	ETag         string    `json:"etag"`
}

// S3Client provides S3 operations against a Garage-compatible object store.
type S3Client interface {
	// PutArtifact uploads a binary artifact.
	PutArtifact(ctx context.Context, bucket, key string, reader io.Reader, contentType string) error

	// GetArtifact returns a ReadCloser for the artifact content. Caller must close.
	GetArtifact(ctx context.Context, bucket, key string) (io.ReadCloser, error)

	// GetPresignedURL returns a time-limited download URL.
	GetPresignedURL(ctx context.Context, bucket, key string, expiry time.Duration) (string, error)

	// ListArtifacts lists objects under a prefix.
	ListArtifacts(ctx context.Context, bucket, prefix string) ([]ArtifactInfo, error)

	// DeleteArtifact removes an object.
	DeleteArtifact(ctx context.Context, bucket, key string) error

	// HeadArtifact checks if an object exists and returns metadata.
	HeadArtifact(ctx context.Context, bucket, key string) (*ArtifactInfo, error)
}

// ArtifactKey builds a canonical S3 key for an artifact.
//
//	ArtifactKey("DockJob", "dockjob-1714500000", "CHEMBL12345-pose1", "pdbqt")
//	=> "DockJob/dockjob-1714500000/CHEMBL12345-pose1.pdbqt"
func ArtifactKey(jobKind, jobName, artifactName, ext string) string {
	return fmt.Sprintf("%s/%s/%s.%s", jobKind, jobName, artifactName, ext)
}

// ---------------------------------------------------------------------------
// GarageClient — real S3 implementation
// ---------------------------------------------------------------------------

// GarageClient implements S3Client using AWS SDK v2 against a Garage endpoint.
type GarageClient struct {
	client    *s3.Client
	presigner *s3.PresignClient
}

// NewGarageClient creates a client configured for the Garage S3-compatible endpoint.
// Uses path-style addressing as required by Garage.
func NewGarageClient(endpoint, accessKey, secretKey, region string) (*GarageClient, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("s3: endpoint is required")
	}
	if accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("s3: access key and secret key are required")
	}
	if region == "" {
		region = "garage"
	}

	// Build a custom AWS config that points to the Garage endpoint with
	// static credentials and path-style addressing.
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(region),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("s3: failed to load AWS config: %w", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})

	return &GarageClient{
		client:    client,
		presigner: s3.NewPresignClient(client),
	}, nil
}

// PutArtifact uploads a binary artifact to the specified bucket and key.
func (g *GarageClient) PutArtifact(ctx context.Context, bucket, key string, reader io.Reader, contentType string) error {
	input := &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        reader,
		ContentType: aws.String(contentType),
	}

	if _, err := g.client.PutObject(ctx, input); err != nil {
		return wrapS3Error(err, "put", bucket, key)
	}
	return nil
}

// GetArtifact retrieves the content of an artifact. The caller must close the returned ReadCloser.
func (g *GarageClient) GetArtifact(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	input := &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}

	result, err := g.client.GetObject(ctx, input)
	if err != nil {
		return nil, wrapS3Error(err, "get", bucket, key)
	}
	return result.Body, nil
}

// GetPresignedURL returns a time-limited download URL for the specified artifact.
func (g *GarageClient) GetPresignedURL(ctx context.Context, bucket, key string, expiry time.Duration) (string, error) {
	input := &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}

	result, err := g.presigner.PresignGetObject(ctx, input, s3.WithPresignExpires(expiry))
	if err != nil {
		return "", wrapS3Error(err, "presign", bucket, key)
	}
	return result.URL, nil
}

// ListArtifacts lists objects under the given prefix in the specified bucket.
func (g *GarageClient) ListArtifacts(ctx context.Context, bucket, prefix string) ([]ArtifactInfo, error) {
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix),
	}

	var artifacts []ArtifactInfo
	paginator := s3.NewListObjectsV2Paginator(g.client, input)

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, wrapS3Error(err, "list", bucket, prefix)
		}

		for _, obj := range page.Contents {
			info := ArtifactInfo{
				Key:  aws.ToString(obj.Key),
				Size: aws.ToInt64(obj.Size),
			}
			if obj.LastModified != nil {
				info.LastModified = *obj.LastModified
			}
			if obj.ETag != nil {
				info.ETag = strings.Trim(*obj.ETag, "\"")
			}
			artifacts = append(artifacts, info)
		}
	}

	if artifacts == nil {
		artifacts = []ArtifactInfo{}
	}
	return artifacts, nil
}

// DeleteArtifact removes an object from the specified bucket.
func (g *GarageClient) DeleteArtifact(ctx context.Context, bucket, key string) error {
	input := &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}

	if _, err := g.client.DeleteObject(ctx, input); err != nil {
		return wrapS3Error(err, "delete", bucket, key)
	}
	return nil
}

// HeadArtifact checks if an object exists and returns its metadata.
func (g *GarageClient) HeadArtifact(ctx context.Context, bucket, key string) (*ArtifactInfo, error) {
	input := &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}

	result, err := g.client.HeadObject(ctx, input)
	if err != nil {
		return nil, wrapS3Error(err, "head", bucket, key)
	}

	info := &ArtifactInfo{
		Key:  key,
		Size: aws.ToInt64(result.ContentLength),
	}
	if result.LastModified != nil {
		info.LastModified = *result.LastModified
	}
	if result.ContentType != nil {
		info.ContentType = *result.ContentType
	}
	if result.ETag != nil {
		info.ETag = strings.Trim(*result.ETag, "\"")
	}
	return info, nil
}

// ---------------------------------------------------------------------------
// NoopS3Client — feature-flag fallback when GARAGE_ENABLED=false
// ---------------------------------------------------------------------------

// NoopS3Client is a no-op implementation of S3Client used when Garage is not
// yet deployed (GARAGE_ENABLED != "true"). All operations log a warning and
// return nil/empty results so the application can run without Garage.
type NoopS3Client struct{}

func (n *NoopS3Client) PutArtifact(_ context.Context, bucket, key string, _ io.Reader, _ string) error {
	log.Printf("[s3/noop] PutArtifact skipped (GARAGE_ENABLED=false): %s/%s", bucket, key)
	return nil
}

func (n *NoopS3Client) GetArtifact(_ context.Context, bucket, key string) (io.ReadCloser, error) {
	log.Printf("[s3/noop] GetArtifact skipped (GARAGE_ENABLED=false): %s/%s", bucket, key)
	return nil, ErrNotFound
}

func (n *NoopS3Client) GetPresignedURL(_ context.Context, bucket, key string, _ time.Duration) (string, error) {
	log.Printf("[s3/noop] GetPresignedURL skipped (GARAGE_ENABLED=false): %s/%s", bucket, key)
	return "", ErrNotFound
}

func (n *NoopS3Client) ListArtifacts(_ context.Context, bucket, prefix string) ([]ArtifactInfo, error) {
	log.Printf("[s3/noop] ListArtifacts skipped (GARAGE_ENABLED=false): %s/%s", bucket, prefix)
	return []ArtifactInfo{}, nil
}

func (n *NoopS3Client) DeleteArtifact(_ context.Context, bucket, key string) error {
	log.Printf("[s3/noop] DeleteArtifact skipped (GARAGE_ENABLED=false): %s/%s", bucket, key)
	return nil
}

func (n *NoopS3Client) HeadArtifact(_ context.Context, bucket, key string) (*ArtifactInfo, error) {
	log.Printf("[s3/noop] HeadArtifact skipped (GARAGE_ENABLED=false): %s/%s", bucket, key)
	return nil, ErrNotFound
}

// ---------------------------------------------------------------------------
// S3 client initialization
// ---------------------------------------------------------------------------

// NewS3ClientFromEnv creates an S3Client based on environment variables.
// If GARAGE_ENABLED is "true", returns a real GarageClient connected to the
// endpoint specified by GARAGE_ENDPOINT, GARAGE_ACCESS_KEY, and GARAGE_SECRET_KEY.
// Otherwise, returns a NoopS3Client that logs warnings.
func NewS3ClientFromEnv() (S3Client, error) {
	if os.Getenv("GARAGE_ENABLED") != "true" {
		log.Println("Garage S3 storage disabled (GARAGE_ENABLED != \"true\"), using no-op client")
		return &NoopS3Client{}, nil
	}

	endpoint := os.Getenv("GARAGE_ENDPOINT")
	accessKey := os.Getenv("GARAGE_ACCESS_KEY")
	secretKey := os.Getenv("GARAGE_SECRET_KEY")
	region := os.Getenv("GARAGE_REGION")

	client, err := NewGarageClient(endpoint, accessKey, secretKey, region)
	if err != nil {
		return nil, fmt.Errorf("failed to create Garage S3 client: %w", err)
	}

	log.Printf("Garage S3 storage enabled (endpoint=%s)", endpoint)
	return client, nil
}

// ---------------------------------------------------------------------------
// Error handling helpers
// ---------------------------------------------------------------------------

// wrapS3Error translates AWS SDK errors into sentinel errors with context.
func wrapS3Error(err error, op, bucket, key string) error {
	if err == nil {
		return nil
	}

	// Check for NoSuchKey.
	var noSuchKey *types.NoSuchKey
	if errors.As(err, &noSuchKey) {
		return fmt.Errorf("s3 %s %s/%s: %w", op, bucket, key, ErrNotFound)
	}

	// Check for NotFound (used by HeadObject when key doesn't exist).
	var notFound *types.NotFound
	if errors.As(err, &notFound) {
		return fmt.Errorf("s3 %s %s/%s: %w", op, bucket, key, ErrNotFound)
	}

	// Check for NoSuchBucket.
	var noSuchBucket *types.NoSuchBucket
	if errors.As(err, &noSuchBucket) {
		return fmt.Errorf("s3 %s %s/%s: %w", op, bucket, key, ErrBucketNotFound)
	}

	// Default: wrap with operation context.
	return fmt.Errorf("s3 %s %s/%s: %w", op, bucket, key, err)
}
