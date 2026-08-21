package v1

import (
	"bytes"
	"context"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/avatar"
)

type avatarStore struct {
	body []byte
	keys []string
}

func (a *avatarStore) PresignPostObject(context.Context, *s3.PutObjectInput, ...func(*s3.PresignPostOptions)) (*s3.PresignedPostRequest, error) {
	return nil, nil
}
func (a *avatarStore) GetObject(_ context.Context, input *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	a.keys = append(a.keys, aws.ToString(input.Key))
	return &s3.GetObjectOutput{
		Body:          io.NopCloser(bytes.NewReader(a.body)),
		ETag:          aws.String(`"d41d8cd98f00b204e9800998ecf8427e"`),
		ContentLength: aws.Int64(int64(len(a.body))),
	}, nil
}
func (a *avatarStore) CopyObject(context.Context, *s3.CopyObjectInput, ...func(*s3.Options)) (*s3.CopyObjectOutput, error) {
	return nil, nil
}
func (a *avatarStore) DeleteObject(context.Context, *s3.DeleteObjectInput, ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	return nil, nil
}

func TestPublicAvatarRead(t *testing.T) {
	const userID = "6f9619ff-8b86-d011-b42d-00cf4fc964ff"
	store := &avatarStore{body: []byte("jpeg-bytes")}
	app := fiber.New()
	RegisterAvatars(app.Group("/v1.0"), avatar.New(store, "avatars"), nil)

	t.Run("serves the object with immutable caching", func(t *testing.T) {
		resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/v1.0/avatars/"+userID+"/4.jpg", nil))
		if err != nil || resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status %v, err %v", resp.StatusCode, err)
		}
		body, _ := io.ReadAll(resp.Body)
		if string(body) != "jpeg-bytes" {
			t.Fatalf("body = %q", body)
		}
		if got := resp.Header.Get(fiber.HeaderCacheControl); got != "public, max-age=31536000, immutable" {
			t.Fatalf("Cache-Control = %q", got)
		}
		if got := resp.Header.Get(fiber.HeaderETag); got != `"d41d8cd98f00b204e9800998ecf8427e"` {
			t.Fatalf("ETag = %q", got)
		}
		if got := resp.Header.Get(fiber.HeaderContentType); got != avatar.PublishedType {
			t.Fatalf("Content-Type = %q", got)
		}
		if got := resp.Header.Get(fiber.HeaderXContentTypeOptions); got != "nosniff" {
			t.Fatalf("X-Content-Type-Options = %q", got)
		}
		// Never a redirect: a 3xx here would hand the caller a presigned URL
		// or the bucket endpoint, turning a public avatar URL into direct
		// storage access.
		if resp.Header.Get(fiber.HeaderLocation) != "" {
			t.Fatal("response carries a Location header")
		}
	})

	// Everything that must 404 without ever reaching S3. up/ is the important
	// one: it holds unvalidated bytes a browser POSTed straight into the
	// bucket, and it must be unaddressable from the public route.
	for _, path := range []string{
		"/v1.0/avatars/" + userID + "/../../up/" + userID + "/1.jpg",
		"/v1.0/avatars/..%2f..%2fup%2f" + userID + "/1.jpg",
		"/v1.0/avatars/" + userID + "/1.png",
		"/v1.0/avatars/" + userID + "/0.jpg",
		"/v1.0/avatars/" + userID + "/-1.jpg",
		"/v1.0/avatars/" + userID + "/1234567890123.jpg",
		"/v1.0/avatars/not-a-uuid/1.jpg",
	} {
		t.Run(path, func(t *testing.T) {
			before := len(store.keys)
			resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, path, nil))
			if err != nil {
				t.Fatalf("err %v", err)
			}
			if resp.StatusCode != fiber.StatusNotFound {
				t.Fatalf("status = %d, want 404", resp.StatusCode)
			}
			if len(store.keys) != before {
				t.Fatalf("reached S3 with key %q", store.keys[len(store.keys)-1])
			}
		})
	}

	for _, key := range store.keys {
		if key != "av/"+userID+"/4.jpg" {
			t.Fatalf("unexpected S3 key %q", key)
		}
	}
}
