package userpurge

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3API is the slice of *s3.Client the purge uses.
type S3API interface {
	ListObjectsV2(ctx context.Context, in *s3.ListObjectsV2Input, opts ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	GetObject(ctx context.Context, in *s3.GetObjectInput, opts ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	PutObject(ctx context.Context, in *s3.PutObjectInput, opts ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	ListObjectVersions(ctx context.Context, in *s3.ListObjectVersionsInput, opts ...func(*s3.Options)) (*s3.ListObjectVersionsOutput, error)
	DeleteObjects(ctx context.Context, in *s3.DeleteObjectsInput, opts ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error)
}

// archive rewrites every action-log archive object of one hand. The archiver
// keys objects by the action-log pk with '#' replaced by '/'. It archives
// only INSERT stream records, so the purge's own rewrites (MODIFY) are not
// archived again.
func (r *run) archive(ctx context.Context, tableID, handID string) error {
	if r.p.cfg.ArchiveBucket == "" {
		return nil
	}
	in := &s3.ListObjectsV2Input{Bucket: aws.String(r.p.cfg.ArchiveBucket), Prefix: aws.String(tableID + "/" + handID + "/")}
	for {
		out, err := r.p.objects.ListObjectsV2(ctx, in)
		if err != nil {
			return fmt.Errorf("userpurge: list archive %s/%s: %w", tableID, handID, err)
		}
		for _, obj := range out.Contents {
			if err := r.archiveObject(ctx, aws.ToString(obj.Key)); err != nil {
				return err
			}
		}
		if !aws.ToBool(out.IsTruncated) {
			return nil
		}
		in.ContinuationToken = out.NextContinuationToken
	}
}

func (r *run) archiveObject(ctx context.Context, objectKey string) error {
	bucket := aws.String(r.p.cfg.ArchiveBucket)
	got, err := r.p.objects.GetObject(ctx, &s3.GetObjectInput{Bucket: bucket, Key: aws.String(objectKey)})
	if err != nil {
		return fmt.Errorf("userpurge: get archive %s: %w", objectKey, err)
	}
	body, err := io.ReadAll(got.Body)
	_ = got.Body.Close()
	if err != nil {
		return fmt.Errorf("userpurge: read archive %s: %w", objectKey, err)
	}
	out, changed, err := anonymizeJSONLines(body, r.sub, r.pseudo)
	if err != nil {
		return fmt.Errorf("userpurge: parse archive %s: %w", objectKey, err)
	}
	if !changed {
		return nil
	}
	if _, err := r.p.objects.PutObject(ctx, &s3.PutObjectInput{Bucket: bucket, Key: aws.String(objectKey), Body: bytes.NewReader(out)}); err != nil {
		return fmt.Errorf("userpurge: put archive %s: %w", objectKey, err)
	}
	r.count("action_log_archive_objects_anonymized", 1)
	return nil
}

// avatars deletes every version and delete marker of the user's avatars
// (both prefixes; the bucket is versioned in prod, inventory §2).
func (r *run) avatars(ctx context.Context) error {
	if r.p.cfg.AvatarBucket == "" {
		return nil
	}
	bucket := aws.String(r.p.cfg.AvatarBucket)
	for _, prefix := range []string{"av/" + r.sub + "/", "up/" + r.sub + "/"} {
		in := &s3.ListObjectVersionsInput{Bucket: bucket, Prefix: aws.String(prefix)}
		for {
			out, err := r.p.objects.ListObjectVersions(ctx, in)
			if err != nil {
				return fmt.Errorf("userpurge: list avatar versions: %w", err)
			}
			var ids []s3types.ObjectIdentifier
			for _, v := range out.Versions {
				ids = append(ids, s3types.ObjectIdentifier{Key: v.Key, VersionId: v.VersionId})
			}
			for _, d := range out.DeleteMarkers {
				ids = append(ids, s3types.ObjectIdentifier{Key: d.Key, VersionId: d.VersionId})
			}
			if len(ids) > 0 {
				res, err := r.p.objects.DeleteObjects(ctx, &s3.DeleteObjectsInput{Bucket: bucket, Delete: &s3types.Delete{Objects: ids, Quiet: aws.Bool(true)}})
				if err != nil {
					return fmt.Errorf("userpurge: delete avatars: %w", err)
				}
				if len(res.Errors) > 0 {
					return fmt.Errorf("userpurge: delete avatar %s: %s", aws.ToString(res.Errors[0].Key), aws.ToString(res.Errors[0].Message))
				}
				r.count("avatar_objects_erased", len(ids))
			}
			if !aws.ToBool(out.IsTruncated) {
				break
			}
			in.KeyMarker, in.VersionIdMarker = out.NextKeyMarker, out.NextVersionIdMarker
		}
	}
	return nil
}
