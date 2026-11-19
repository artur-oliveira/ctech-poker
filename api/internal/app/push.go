package app

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"gopkg.aoctech.app/api-commons/awsconfig"
	"gopkg.aoctech.app/api-commons/pushsvc"

	v1 "gopkg.aoctech.app/poker/api/internal/api/v1"
	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/pokerpush"
)

// offSender is the PushJobSender when no sender function is configured: the test route then answers an error instead of
// invoking nothing.
type offSender struct{}

func (offSender) Send(context.Context, pushsvc.Job) error {
	return errors.New("push: PUSH_SENDER_FUNCTION is not set")
}

// newPushDeps builds the Web Push routes' dependencies. Without PUSH_VAPID_PUBLIC_KEY the service is off (config answers
// enabled=false, subscribing answers 503), so a deploy before the operator writes the keys changes nothing.
func newPushDeps(cfg *config.Config, db *dynamodb.Client) (*v1.PushDeps, error) {
	prefs := pokerpush.NewService(pushsvc.NewReminderStore(db, cfg.Env))
	svc := pushsvc.NewService(pushsvc.NewStore(db, cfg.Env), cfg.PushVAPIDPublicKey).WithOnSubscribe(prefs.SeedOnSubscribe)
	var sender v1.PushJobSender = offSender{}
	if cfg.PushSenderFunction != "" {
		awsCfg, err := awsconfig.Load(context.Background(), cfg.AWSRegion)
		if err != nil {
			return nil, err
		}
		sender = pushsvc.NewInvoker(lambda.NewFromConfig(awsCfg), cfg.PushSenderFunction)
	}
	return &v1.PushDeps{Service: svc, Prefs: prefs, Sender: sender}, nil
}
