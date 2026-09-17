package config

import (
	"fmt"
	"strconv"
	"strings"
)

// Backup says where `ddcore backup` writes its archive and, optionally, which
// bucket it copies it to. It is read from the environment only: a destination
// differs per machine and its credentials are secrets.
type Backup struct {
	// Dir is where an archive is written locally; <dataDir>/backups by default.
	Dir string
	// S3 is the bucket `--to s3` uploads to. Every field left unset is taken
	// from DDCORE_S3_*, so a site already storing files in a bucket only has
	// to say so; Prefix defaults to that storage prefix plus "/backups".
	S3 S3
	// S3Configured reports whether a bucket is known at all.
	S3Configured bool
	// Keep is how many archives `--keep` leaves in place when it is not given
	// on the command line; zero keeps all of them.
	Keep int
}

func backupFromEnv(st Storage) (Backup, error) {
	b := Backup{Dir: env("DDCORE_BACKUP_DIR", "")}
	s := st.S3
	s.Endpoint = env("DDCORE_BACKUP_S3_ENDPOINT", s.Endpoint)
	s.Region = env("DDCORE_BACKUP_S3_REGION", s.Region)
	s.Bucket = env("DDCORE_BACKUP_S3_BUCKET", s.Bucket)
	s.AccessKey = env("DDCORE_BACKUP_S3_ACCESS_KEY", s.AccessKey)
	s.SecretKey = env("DDCORE_BACKUP_S3_SECRET_KEY", s.SecretKey)
	s.UseSSL = envBool("DDCORE_BACKUP_S3_USE_SSL", s.UseSSL)
	s.PathStyle = envBool("DDCORE_BACKUP_S3_PATH_STYLE", s.PathStyle)
	def := "backups"
	if st.S3.Prefix != "" {
		def = st.S3.Prefix + "/backups"
	}
	s.Prefix = strings.Trim(env("DDCORE_BACKUP_S3_PREFIX", def), "/")
	b.S3 = s
	b.S3Configured = s.Bucket != "" && s.AccessKey != "" && s.SecretKey != ""
	if strings.Contains(s.Endpoint, "://") {
		return b, fmt.Errorf("DDCORE_BACKUP_S3_ENDPOINT: %q must be a host, without a scheme", s.Endpoint)
	}
	if v := env("DDCORE_BACKUP_KEEP", ""); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return b, fmt.Errorf("DDCORE_BACKUP_KEEP: %q is not a number of archives", v)
		}
		b.Keep = n
	}
	return b, nil
}
