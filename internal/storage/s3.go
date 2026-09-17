package storage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/jrvidotti/ddcore/internal/config"
)

// S3 keeps files in an S3-compatible bucket. The bucket is expected to be
// private: a download is answered with a redirect to a presigned URL, issued
// only after the server has checked the permission.
type S3 struct {
	client *minio.Client
	cfg    config.S3
}

func NewS3(_ context.Context, cfg config.S3) (*S3, error) {
	lookup := minio.BucketLookupAuto
	if cfg.PathStyle {
		lookup = minio.BucketLookupPath
	}
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:        credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure:       cfg.UseSSL,
		Region:       cfg.Region,
		BucketLookup: lookup,
	})
	if err != nil {
		return nil, fmt.Errorf("storage: s3: %w", err)
	}
	return &S3{client: client, cfg: cfg}, nil
}

func (s *S3) Backend() string { return "s3" }

func (s *S3) object(key string) string {
	if s.cfg.Prefix == "" {
		return key
	}
	return s.cfg.Prefix + "/" + key
}

func notFound(err error) bool {
	code := minio.ToErrorResponse(err).Code
	return code == "NoSuchKey" || code == "NotFound"
}

func (s *S3) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	_, err := s.client.PutObject(ctx, s.cfg.Bucket, s.object(key), r, size, minio.PutObjectOptions{ContentType: contentType})
	return err
}

func (s *S3) Open(ctx context.Context, key string) (io.ReadCloser, Info, error) {
	st, err := s.client.StatObject(ctx, s.cfg.Bucket, s.object(key), minio.StatObjectOptions{})
	if err != nil {
		if notFound(err) {
			return nil, Info{}, ErrNotFound
		}
		return nil, Info{}, err
	}
	obj, err := s.client.GetObject(ctx, s.cfg.Bucket, s.object(key), minio.GetObjectOptions{})
	if err != nil {
		return nil, Info{}, err
	}
	return obj, Info{Size: st.Size, ModTime: st.LastModified}, nil
}

func (s *S3) Delete(ctx context.Context, key string) error {
	err := s.client.RemoveObject(ctx, s.cfg.Bucket, s.object(key), minio.RemoveObjectOptions{})
	if err != nil && notFound(err) {
		return nil
	}
	return err
}

// Serve redirects to a presigned URL. The response overrides make the bucket
// announce the same type and disposition the local backend would, whatever
// content type the uploader claimed when the object was stored.
func (s *S3) Serve(w http.ResponseWriter, r *http.Request, key string, sv Serving) error {
	if _, err := s.client.StatObject(r.Context(), s.cfg.Bucket, s.object(key), minio.StatObjectOptions{}); err != nil {
		if notFound(err) {
			return ErrNotFound
		}
		return err
	}
	params := url.Values{}
	params.Set("response-content-disposition", disposition(sv))
	params.Set("response-content-type", servedType(sv))
	u, err := s.client.PresignedGetObject(r.Context(), s.cfg.Bucket, s.object(key), s.cfg.PresignTTL, params)
	if err != nil {
		return err
	}
	// the link expires, so neither the browser nor a proxy may keep the redirect
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, u.String(), http.StatusFound)
	return nil
}

// List pages through the bucket under the configured prefix, handing back keys
// with that prefix stripped so they mean the same thing on both backends.
func (s *S3) List(ctx context.Context, prefix string, fn func(key string, info Info) error) error {
	strip := ""
	if s.cfg.Prefix != "" {
		strip = s.cfg.Prefix + "/"
	}
	// cancelled on return, so stopping early does not leave the lister's
	// goroutine blocked on a channel nobody reads
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	for obj := range s.client.ListObjects(ctx, s.cfg.Bucket, minio.ListObjectsOptions{Prefix: strip + prefix, Recursive: true}) {
		if obj.Err != nil {
			return obj.Err
		}
		if err := fn(strings.TrimPrefix(obj.Key, strip), Info{Size: obj.Size, ModTime: obj.LastModified}); err != nil {
			return err
		}
	}
	return nil
}
