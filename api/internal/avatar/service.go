package avatar

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

const (
	MaxBytes      int64 = 2 * 1024 * 1024
	MaxDimension        = 4096
	HeaderRange         = "bytes=0-65535"
	PublishedType       = "image/jpeg"
)

var (
	ErrNotFound      = errors.New("avatar upload not found")
	ErrInvalidImage  = errors.New("invalid avatar image")
	ErrImageTooLarge = errors.New("avatar image is too large")
	ErrEXIF          = errors.New("avatar image contains EXIF metadata")
)

type API interface {
	PresignPostObject(context.Context, *s3.PutObjectInput, ...func(*s3.PresignPostOptions)) (*s3.PresignedPostRequest, error)
	GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	CopyObject(context.Context, *s3.CopyObjectInput, ...func(*s3.Options)) (*s3.CopyObjectOutput, error)
	DeleteObject(context.Context, *s3.DeleteObjectInput, ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
}

type Service struct {
	api    API
	bucket string
}

func New(api API, bucket string) *Service { return &Service{api: api, bucket: bucket} }
func (s *Service) Enabled() bool          { return s != nil && s.api != nil && s.bucket != "" }

type Upload struct {
	URL    string            `json:"url"`
	Fields map[string]string `json:"fields"`
}

func (s *Service) Presign(ctx context.Context, key string) (*Upload, error) {
	if !s.Enabled() {
		return nil, errors.New("avatar uploads are disabled")
	}
	request, err := s.api.PresignPostObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key), ContentType: aws.String(PublishedType),
	}, func(options *s3.PresignPostOptions) {
		options.Expires = 2 * time.Minute
		options.Conditions = []any{
			[]any{"content-length-range", 1, MaxBytes},
			map[string]string{"Content-Type": PublishedType},
		}
	})
	if err != nil {
		return nil, fmt.Errorf("avatar: presign post: %w", err)
	}
	fields := make(map[string]string, len(request.Values)+1)
	for key, value := range request.Values {
		fields[key] = value
	}
	fields["Content-Type"] = PublishedType
	return &Upload{URL: request.URL, Fields: fields}, nil
}

func (s *Service) ValidateAndPublish(ctx context.Context, uploadKey, publishedKey string) error {
	object, err := s.api.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(uploadKey), Range: aws.String(HeaderRange),
	})
	if err != nil {
		var notFound interface{ ErrorCode() string }
		if errors.As(err, &notFound) && (notFound.ErrorCode() == "NoSuchKey" || notFound.ErrorCode() == "NotFound") {
			return ErrNotFound
		}
		return fmt.Errorf("avatar: get quarantine object: %w", err)
	}
	defer object.Body.Close()
	if totalSize(object.ContentRange, object.ContentLength) > MaxBytes {
		return ErrImageTooLarge
	}
	header, err := io.ReadAll(io.LimitReader(object.Body, 65536))
	if err != nil {
		return fmt.Errorf("avatar: read header: %w", err)
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(header))
	if err != nil || (format != "jpeg" && format != "png") {
		return ErrInvalidImage
	}
	if config.Width < 1 || config.Height < 1 || config.Width > MaxDimension || config.Height > MaxDimension {
		return ErrImageTooLarge
	}
	if format == "jpeg" && hasEXIF(header) {
		return ErrEXIF
	}
	_, err = s.api.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(publishedKey),
		CopySource:        aws.String(url.PathEscape(s.bucket + "/" + uploadKey)),
		MetadataDirective: types.MetadataDirectiveReplace,
		ContentType:       aws.String(PublishedType),
		CacheControl:      aws.String("public, max-age=31536000, immutable"),
	})
	if err != nil {
		return fmt.Errorf("avatar: publish object: %w", err)
	}
	return nil
}

func (s *Service) DeleteBestEffort(ctx context.Context, keys ...string) {
	for _, key := range keys {
		if key == "" || !s.Enabled() {
			continue
		}
		if _, err := s.api.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)}); err != nil {
			slog.Warn("avatar: object cleanup failed", "key", key, "err", err)
		}
	}
}

func totalSize(contentRange *string, fallback *int64) int64 {
	if contentRange != nil {
		if slash := strings.LastIndexByte(*contentRange, '/'); slash >= 0 {
			if size, err := strconv.ParseInt((*contentRange)[slash+1:], 10, 64); err == nil {
				return size
			}
		}
	}
	return aws.ToInt64(fallback)
}

func hasEXIF(data []byte) bool {
	if len(data) < 4 || data[0] != 0xff || data[1] != 0xd8 {
		return false
	}
	for offset := 2; offset+4 <= len(data); {
		if data[offset] != 0xff {
			offset++
			continue
		}
		marker := data[offset+1]
		if marker == 0xda || marker == 0xd9 {
			return false
		}
		if marker == 0x01 || (marker >= 0xd0 && marker <= 0xd7) {
			offset += 2
			continue
		}
		length := int(data[offset+2])<<8 | int(data[offset+3])
		if length < 2 || offset+2+length > len(data) {
			return false
		}
		if marker == 0xe1 && length >= 8 && bytes.Equal(data[offset+4:offset+10], []byte("Exif\x00\x00")) {
			return true
		}
		offset += 2 + length
	}
	return false
}
