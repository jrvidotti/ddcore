package config

import (
	"fmt"
	"strings"
	"time"
)

// Storage backends.
const (
	StorageLocal = "local" // <dataDir>/files, the default
	StorageS3    = "s3"    // any S3-compatible bucket: AWS, R2, MinIO, B2
)

// Storage is where uploaded bytes live. Like Mail it is a deployment decision
// — a bucket and its credentials differ on every machine and one of them is a
// secret — so it is read from the environment only.
type Storage struct {
	Backend string
	S3      S3
}

// S3 locates a bucket and signs requests to it.
type S3 struct {
	Endpoint  string // host[:port], no scheme
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
	// Prefix is prepended to every object key, so one bucket can hold
	// several sites.
	Prefix string
	UseSSL bool
	// PathStyle addresses the bucket as endpoint/bucket rather than
	// bucket.endpoint, which MinIO and most self-hosted servers need.
	PathStyle bool
	// PresignTTL is how long a download link handed to a browser stays valid.
	PresignTTL time.Duration
}

// DefaultPresignTTL is short on purpose: the link is issued right after the
// permission check, and anyone holding it can use it until it expires.
const DefaultPresignTTL = 5 * time.Minute

func storageFromEnv() (Storage, error) {
	s := Storage{
		Backend: strings.ToLower(env("DDCORE_STORAGE", StorageLocal)),
		S3: S3{
			Endpoint:   env("DDCORE_S3_ENDPOINT", "s3.amazonaws.com"),
			Region:     env("DDCORE_S3_REGION", ""),
			Bucket:     env("DDCORE_S3_BUCKET", ""),
			AccessKey:  env("DDCORE_S3_ACCESS_KEY", ""),
			SecretKey:  env("DDCORE_S3_SECRET_KEY", ""),
			Prefix:     strings.Trim(env("DDCORE_S3_PREFIX", ""), "/"),
			UseSSL:     envBool("DDCORE_S3_USE_SSL", true),
			PathStyle:  envBool("DDCORE_S3_PATH_STYLE", false),
			PresignTTL: DefaultPresignTTL,
		},
	}
	if v := env("DDCORE_S3_PRESIGN_TTL", ""); v != "" {
		d, err := time.ParseDuration(v)
		// S3 refuses a presigned URL valid for more than seven days
		if err != nil || d <= 0 || d > 7*24*time.Hour {
			return s, fmt.Errorf("DDCORE_S3_PRESIGN_TTL: %q is not a duration between 1s and 168h", v)
		}
		s.S3.PresignTTL = d
	}
	switch s.Backend {
	case StorageLocal:
	case StorageS3:
		if s.S3.Bucket == "" {
			return s, fmt.Errorf("DDCORE_STORAGE=s3 needs DDCORE_S3_BUCKET")
		}
		if s.S3.AccessKey == "" || s.S3.SecretKey == "" {
			return s, fmt.Errorf("DDCORE_STORAGE=s3 needs DDCORE_S3_ACCESS_KEY and DDCORE_S3_SECRET_KEY")
		}
		if strings.Contains(s.S3.Endpoint, "://") {
			return s, fmt.Errorf("DDCORE_S3_ENDPOINT: %q must be a host, without a scheme (use DDCORE_S3_USE_SSL)", s.S3.Endpoint)
		}
	default:
		return s, fmt.Errorf("DDCORE_STORAGE: %q is not local or s3", s.Backend)
	}
	return s, nil
}
