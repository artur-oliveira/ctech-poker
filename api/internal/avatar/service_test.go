package avatar

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"io"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type fakeS3 struct {
	body      []byte
	copyInput *s3.CopyObjectInput
}

func (f *fakeS3) PresignPostObject(context.Context, *s3.PutObjectInput, ...func(*s3.PresignPostOptions)) (*s3.PresignedPostRequest, error) {
	return &s3.PresignedPostRequest{URL: "https://bucket.s3.dualstack.us-east-1.amazonaws.com", Values: map[string]string{"key": "up/u/1.jpg"}}, nil
}
func (f *fakeS3) GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	rangeHeader := "bytes 0-99/100"
	return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(f.body)), ContentRange: &rangeHeader,
		ContentLength: aws.Int64(int64(len(f.body)))}, nil
}
func (f *fakeS3) CopyObject(_ context.Context, input *s3.CopyObjectInput, _ ...func(*s3.Options)) (*s3.CopyObjectOutput, error) {
	f.copyInput = input
	return &s3.CopyObjectOutput{}, nil
}
func (f *fakeS3) DeleteObject(context.Context, *s3.DeleteObjectInput, ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	return &s3.DeleteObjectOutput{}, nil
}

func jpegHeader(width, height int, exif bool) []byte {
	var encoded bytes.Buffer
	_ = jpeg.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 1, 1)), nil)
	data := encoded.Bytes()
	sof := bytes.Index(data, []byte{0xff, 0xc0})
	data[sof+5], data[sof+6] = byte(height>>8), byte(height)
	data[sof+7], data[sof+8] = byte(width>>8), byte(width)
	if exif {
		data = append(append(append([]byte{}, data[:2]...), 0xff, 0xe1, 0x00, 0x08, 'E', 'x', 'i', 'f', 0x00, 0x00), data[2:]...)
	}
	return data
}

func TestValidateAndPublishRejectsDimensionBeforeCopy(t *testing.T) {
	fake := &fakeS3{body: jpegHeader(5000, 5000, false)}
	err := New(fake, "avatars").ValidateAndPublish(context.Background(), "up/u/1.jpg", "av/u/1.jpg")
	if err != ErrImageTooLarge {
		t.Fatalf("got %v", err)
	}
	if fake.copyInput != nil {
		t.Fatal("unsafe image was copied")
	}
}

func TestValidateAndPublishRejectsHostileFormatsAndEXIF(t *testing.T) {
	for name, body := range map[string][]byte{
		"gif": []byte("GIF89a"), "webp": []byte("RIFFxxxxWEBP"),
		"svg": []byte(`<svg onload="alert(1)"/>`), "html": []byte(`<script>alert(1)</script>`),
	} {
		t.Run(name, func(t *testing.T) {
			fake := &fakeS3{body: body}
			if err := New(fake, "avatars").ValidateAndPublish(context.Background(), "up/u/1.jpg", "av/u/1.jpg"); err != ErrInvalidImage {
				t.Fatalf("got %v", err)
			}
			if fake.copyInput != nil {
				t.Fatal("hostile body was copied")
			}
		})
	}
	fake := &fakeS3{body: jpegHeader(192, 192, true)}
	if err := New(fake, "avatars").ValidateAndPublish(context.Background(), "up/u/1.jpg", "av/u/1.jpg"); err != ErrEXIF {
		t.Fatalf("got %v", err)
	}
	if fake.copyInput != nil {
		t.Fatal("EXIF image was copied")
	}
}

func TestValidateAndPublishReplacesUntrustedMetadata(t *testing.T) {
	fake := &fakeS3{body: jpegHeader(192, 192, false)}
	if err := New(fake, "avatars").ValidateAndPublish(context.Background(), "up/u/1.jpg", "av/u/1.jpg"); err != nil {
		t.Fatal(err)
	}
	if fake.copyInput == nil || fake.copyInput.MetadataDirective != "REPLACE" || aws.ToString(fake.copyInput.ContentType) != PublishedType {
		t.Fatalf("unsafe copy parameters: %+v", fake.copyInput)
	}
	if aws.ToString(fake.copyInput.CacheControl) != "public, max-age=31536000, immutable" {
		t.Fatal("immutable cache missing")
	}
}

func TestPresignIncludesRequiredFormType(t *testing.T) {
	upload, err := New(&fakeS3{}, "avatars").Presign(context.Background(), "up/u/1.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if upload.Fields["Content-Type"] != PublishedType {
		t.Fatalf("fields: %#v", upload.Fields)
	}
	if !bytes.Contains([]byte(upload.URL), []byte("dualstack")) {
		t.Fatalf("URL is not dualstack: %s", upload.URL)
	}
}
