package v1

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// stubDynamoClient returns a DynamoDB client pointed at an in-process server
// that answers every call with an empty result set.
//
// Route-level tests here only assert "this endpoint is wired and shaped
// right", but the handlers behind them do reach a store now (the catalog
// endpoints read entitlements to report ownership). A nil *dynamodb.Client
// panics the moment anything touches it, so give those tests a client that
// is real enough to run the query path and boring enough to have no rows.
func stubDynamoClient(t *testing.T) *dynamodb.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		_, _ = w.Write([]byte(`{"Items":[],"Count":0,"ScannedCount":0}`))
	}))
	t.Cleanup(srv.Close)
	cfg := aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("dummy", "dummy", ""),
	}
	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) { o.BaseEndpoint = aws.String(srv.URL) })
}
