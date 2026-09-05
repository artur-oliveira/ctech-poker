//go:build integration

package social

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

func TestDynamoConcurrentCrossedRequestsNeverLeaveUnilateralFriendship(t *testing.T) {
	db := socialTestClient(t)
	env := fmt.Sprintf("social_test_%d", time.Now().UnixNano())
	name := env + "_" + tableSocialEdges
	createSocialEdgeTestTable(t, db, name)
	store := NewStore(db, env)
	service := NewService(store, true)
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, pair := range [][2]string{{"a", "b"}, {"b", "a"}} {
		pair := pair
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, requestErr := service.Request(context.Background(), pair[0], pair[1], pair[0])
			errs <- requestErr
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for requestErr := range errs {
		if requestErr != nil {
			t.Fatal(requestErr)
		}
	}
	a, err := store.Get(context.Background(), "a", "b")
	if err != nil {
		t.Fatal(err)
	}
	b, err := store.Get(context.Background(), "b", "a")
	if err != nil {
		t.Fatal(err)
	}
	assertPair(t, a, b, RelationshipFriend, RelationshipFriend)
}

func socialTestClient(t *testing.T) *dynamodb.Client {
	t.Helper()
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")),
	)
	if err != nil {
		t.Fatal(err)
	}
	return dynamodb.NewFromConfig(cfg, func(options *dynamodb.Options) {
		options.BaseEndpoint = aws.String("http://localhost:8555")
	})
}
