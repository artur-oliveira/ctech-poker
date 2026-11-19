// Command push-tick is the scheduled Lambda behind Poker's Web Push reminders. EventBridge Scheduler runs it every 15
// minutes (São Paulo time); it reads the reminders due in the slot, applies the Poker rules (pokerpush) and hands each push
// to the push-sender Lambda. It holds no VAPID key: sending is the sender's job. docs/specs/2026-11-18-scheduled-web-push.md.
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"

	"gopkg.aoctech.app/api-commons/pushsvc"
	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/dailyreward"
	"gopkg.aoctech.app/poker/api/internal/pokerpush"
	"gopkg.aoctech.app/poker/api/internal/sessionlog"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	lambda.Start(handler)
}

func handler(ctx context.Context) error {
	cfg, err := config.LoadForLambda()
	if err != nil {
		return err
	}
	if cfg.PushSenderFunction == "" {
		slog.WarnContext(ctx, "push tick: PUSH_SENDER_FUNCTION is not set; nothing sent")
		return nil
	}
	awsCfg, err := awscfg.LoadDefaultConfig(ctx)
	if err != nil {
		return err
	}
	db := dynamodb.NewFromConfig(awsCfg)
	reminders := pushsvc.NewReminderStore(db, cfg.Env)
	sessions := pokerpush.SessionLog{Store: sessionlog.NewStore(db, cfg.Env)}
	// The wallet is nil on purpose: the tick only reads Status, it never claims a reward.
	status := dailyreward.NewService(nil, dailyreward.NewStore(db, cfg.Env))
	rules := pokerpush.NewRules(status, sessions, reminders)
	invoker := pushsvc.NewInvoker(awslambda.NewFromConfig(awsCfg), cfg.PushSenderFunction)
	return NewTick(reminders, pushsvc.NewStore(db, cfg.Env), rules, invoker, sessions).Run(ctx, time.Now())
}
