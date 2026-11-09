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

	if s.S3.Fallback {
		t.Errorf("a fallback nobody set: %+v", s.S3)
	}
	for v, want := range map[string]string{"/prod/": "prod", "/": ""} {
		t.Setenv("DDCORE_S3_FALLBACK_PREFIX", v)
		s, err = storageFromEnv()
		if err != nil || !s.S3.Fallback || s.S3.FallbackPrefix != want {
			t.Errorf("fallback %q: %+v %v", v, s.S3, err)
		}
		b, err := backupFromEnv(s)
		if err != nil || b.S3.Fallback {
			t.Errorf("the backup bucket inherited the fallback: %+v %v", b.S3, err)
		}
	}
	t.Setenv("DDCORE_S3_FALLBACK_PREFIX", "")

	for k, v := range map[string]string{"DDCORE_S3_ENDPOINT": "https://minio:9000", "DDCORE_S3_PRESIGN_TTL": "30d", "DDCORE_STORAGE": "gcs",
		"DDCORE_S3_FALLBACK_PREFIX": "site-a/"} {
		t.Run(k, func(t *testing.T) {
			t.Setenv(k, v)
			if _, err := storageFromEnv(); err == nil {
				t.Errorf("%s=%s accepted", k, v)
			}
		})
	}
}

// A fallback prefix only means something on a bucket.
func TestStorageFallbackNeedsS3(t *testing.T) {
	t.Setenv("DDCORE_S3_FALLBACK_PREFIX", "prod")
	if _, err := storageFromEnv(); err == nil || !strings.Contains(err.Error(), "DDCORE_STORAGE=s3") {
		t.Errorf("fallback on local: %v", err)
	}
}
