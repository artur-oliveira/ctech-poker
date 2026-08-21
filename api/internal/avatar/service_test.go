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
	getInput  *s3.GetObjectInput
}

func (f *fakeS3) PresignPostObject(context.Context, *s3.PutObjectInput, ...func(*s3.PresignPostOptions)) (*s3.PresignedPostRequest, error) {
	return &s3.PresignedPostRequest{URL: "https://bucket.s3.dualstack.us-east-1.amazonaws.com", Values: map[string]string{"key": "up/u/1.jpg"}}, nil
}
func (f *fakeS3) GetObject(_ context.Context, input *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	f.getInput = input
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

// The public read route feeds a URL path segment into an S3 key. These are the
// keys it must refuse to build at all: anything that would escape av/, and in
// particular anything that would land in the up/ quarantine, where unvalidated
// bytes a browser POSTed sit for up to a day.
func TestKeyBuildersRejectAnythingButAUUID(t *testing.T) {
	const valid = "6f9619ff-8b86-d011-b42d-00cf4fc964ff"
	hostile := []string{
		"../up/6f9619ff-8b86-d011-b42d-00cf4fc964ff",
		"..%2Fup%2F6f9619ff-8b86-d011-b42d-00cf4fc964ff",
		"6f9619ff-8b86-d011-b42d-00cf4fc964ff/../../up/6f9619ff-8b86-d011-b42d-00cf4fc964ff",
		"6f9619ff-8b86-d011-b42d-00cf4fc964ff/..",
		"", "..", "/", "av", "up",
		"6f9619ff-8b86-d011-b42d-00cf4fc964f",  // one hex digit short
		"6f9619ff8b86d011b42d00cf4fc964ff",     // no dashes
		"6f9619ff-8b86-d011-b42d-00cf4fc964fg", // g is not hex
	}
	for _, userID := range hostile {
		if key, err := PublishedKey(userID, 1); err != ErrInvalidKey {
			t.Fatalf("PublishedKey(%q) = %q, %v; want ErrInvalidKey", userID, key, err)
		}
		if key, err := UploadKey(userID, 1); err != ErrInvalidKey {
			t.Fatalf("UploadKey(%q) = %q, %v; want ErrInvalidKey", userID, key, err)
		}
	}
	for _, version := range []int{0, -1, -1000} {
		if _, err := PublishedKey(valid, version); err != ErrInvalidKey {
			t.Fatalf("PublishedKey(version=%d) = %v; want ErrInvalidKey", version, err)
		}
	}
	key, err := PublishedKey(valid, 7)
	if err != nil || key != "av/"+valid+"/7.jpg" {
		t.Fatalf("PublishedKey = %q, %v", key, err)
	}
	key, err = UploadKey(valid, 7)
	if err != nil || key != "up/"+valid+"/7.jpg" {
		t.Fatalf("UploadKey = %q, %v", key, err)
	}
}

// Get must always read from av/, whatever it is handed.
func TestGetReadsOnlyThePublishedPrefix(t *testing.T) {
	fake := &fakeS3{body: jpegHeader(192, 192, false)}
	service := New(fake, "avatars")
	object, err := service.Get(context.Background(), "6f9619ff-8b86-d011-b42d-00cf4fc964ff", 3)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	object.Body.Close()
	if got := aws.ToString(fake.getInput.Key); got != "av/6f9619ff-8b86-d011-b42d-00cf4fc964ff/3.jpg" {
		t.Fatalf("read key = %q", got)
	}
	if _, err := service.Get(context.Background(), "../up/x", 3); err != ErrInvalidKey {
		t.Fatalf("hostile userID reached S3: %v", err)
	}
}
