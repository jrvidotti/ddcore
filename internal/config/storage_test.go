package config

import (
	"strings"
	"testing"
	"time"
)

func TestStorageFromEnv(t *testing.T) {
	s, err := storageFromEnv()
	if err != nil || s.Backend != StorageLocal {
		t.Fatalf("default: %+v %v", s, err)
	}

	t.Setenv("DDCORE_STORAGE", "s3")
	if _, err := storageFromEnv(); err == nil || !strings.Contains(err.Error(), "DDCORE_S3_BUCKET") {
		t.Fatalf("s3 without a bucket: %v", err)
	}
	t.Setenv("DDCORE_S3_BUCKET", "b")
	if _, err := storageFromEnv(); err == nil || !strings.Contains(err.Error(), "ACCESS_KEY") {
		t.Fatalf("s3 without credentials: %v", err)
	}
	t.Setenv("DDCORE_S3_ACCESS_KEY", "k")
	t.Setenv("DDCORE_S3_SECRET_KEY", "s")
	t.Setenv("DDCORE_S3_PREFIX", "/site-a/")
	t.Setenv("DDCORE_S3_PRESIGN_TTL", "90s")
	s, err = storageFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if s.S3.Prefix != "site-a" || s.S3.PresignTTL != 90*time.Second || !s.S3.UseSSL {
		t.Errorf("s3: %+v", s.S3)
	}

	for k, v := range map[string]string{"DDCORE_S3_ENDPOINT": "https://minio:9000", "DDCORE_S3_PRESIGN_TTL": "30d", "DDCORE_STORAGE": "gcs"} {
		t.Run(k, func(t *testing.T) {
			t.Setenv(k, v)
			if _, err := storageFromEnv(); err == nil {
				t.Errorf("%s=%s accepted", k, v)
			}
		})
	}
}
