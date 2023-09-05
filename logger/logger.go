package logger

import (
	"fmt"
	"github.com/TheZeroSlave/zapsentry"
	"github.com/getsentry/sentry-go"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"orchdio/blueprint"
	"os"
	"time"
)

// NewLogger returns a new zap logger
func NewLogger() *zap.Logger {
	logger, _ := zap.NewProduction()
	defer func(logger *zap.Logger) {
		err := logger.Sync()
		if err != nil {
			panic(err)
		}
	}(logger)

	return logger
}

// NewZapSentryLogger returns a new zap logger with sentry integration
func NewZapSentryLogger(orchdioLoggerOpts *blueprint.OrchdioLoggerOptions) *zap.Logger {

	if orchdioLoggerOpts == nil {
		orchdioLoggerOpts = &blueprint.OrchdioLoggerOptions{
			ApplicationPublicKey: "NOT_SET",
			RequestID:            "NOT_SET",
		}
	}

	if orchdioLoggerOpts.ApplicationPublicKey == "" {
		orchdioLoggerOpts.ApplicationPublicKey = "NOT_SET"
	}

	if orchdioLoggerOpts.RequestID == "" {
		orchdioLoggerOpts.RequestID = "not_set"
	}

	sentryTrace := false
	if orchdioLoggerOpts.AddTrace == true {
		sentryTrace = true
	}

	cfg := zapsentry.Configuration{
		Level:             zapcore.WarnLevel,
		BreadcrumbLevel:   zapcore.WarnLevel,
		EnableBreadcrumbs: true,
		DisableStacktrace: !sentryTrace,
		Tags: map[string]string{
			"component":  "system",
			"when":       time.Now().String(),
			"public_key": orchdioLoggerOpts.ApplicationPublicKey,
			"request_id": orchdioLoggerOpts.RequestID,
		},
	}

	log, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}

	sentryClient, sErr := sentry.NewClient(sentry.ClientOptions{
		Dsn:              os.Getenv("SENTRY_DSN"),
		TracesSampleRate: 1.0,
		AttachStacktrace: sentryTrace,
	})

	defer sentryClient.Flush(2)

	if sErr != nil {
		fmt.Println("error creating sentry client")
		panic(err)
	}

	core, zErr := zapsentry.NewCore(cfg, zapsentry.NewSentryClientFromDSN(os.Getenv("SENTRY_DSN")))
	if zErr != nil {
		fmt.Println("error creating zap core")
	}

	log = zapsentry.AttachCoreToLogger(core, log)
	sentryScope := sentry.NewScope()

	if orchdioLoggerOpts.ApplicationPublicKey != "" {
		sentryScope.AddBreadcrumb(&sentry.Breadcrumb{
			Category:  "Application (public key)",
			Message:   "Request made by application",
			Data:      map[string]interface{}{"application": orchdioLoggerOpts.ApplicationPublicKey},
			Timestamp: time.Time{},
		}, 1)
	}

	if orchdioLoggerOpts.RequestID != "" {
		sentryScope.AddBreadcrumb(&sentry.Breadcrumb{
			Category:  "Request ID",
			Data:      map[string]interface{}{"request_id": orchdioLoggerOpts.RequestID},
			Timestamp: time.Time{},
		}, 1)
	}

	if orchdioLoggerOpts.AppID != "" {
		sentryScope.AddBreadcrumb(&sentry.Breadcrumb{
			Category:  "App ID",
			Message:   "ID of application making the request",
			Data:      map[string]interface{}{"app_id": orchdioLoggerOpts.AppID},
			Timestamp: time.Time{},
		}, 1)
	}

	if orchdioLoggerOpts.Entity != "" {
		sentryScope.AddBreadcrumb(&sentry.Breadcrumb{
			Category:  "Entity",
			Message:   "Entity that the user wants to interact with",
			Data:      map[string]interface{}{"entity_id": orchdioLoggerOpts.AppID},
			Timestamp: time.Time{},
		}, 1)
	}

	if orchdioLoggerOpts.Error != nil {
		sentryScope.AddBreadcrumb(&sentry.Breadcrumb{
			Category:  "Error",
			Message:   "Error encountered while making the request",
			Data:      map[string]interface{}{"error": orchdioLoggerOpts.Error},
			Timestamp: time.Time{},
		}, 1)
	}

	if orchdioLoggerOpts.Platform != "" {
		sentryScope.AddBreadcrumb(&sentry.Breadcrumb{
			Category:  "Platform",
			Message:   "Platform that the user is interacting with",
			Data:      map[string]interface{}{"platform": orchdioLoggerOpts.Platform},
			Timestamp: time.Time{},
		}, 1)
	}

	sentryScope.AddBreadcrumb(&sentry.Breadcrumb{
		Message:   "",
		Data:      nil,
		Level:     "",
		Timestamp: time.Time{},
	}, 1)

	return log.With(zapsentry.NewScopeFromScope(sentryScope))
}

func NewLoggerWithConfig(config zap.Config) *zap.Logger {
	logger, _ := config.Build()
	defer func(logger *zap.Logger) {
		err := logger.Sync()
		if err != nil {
			panic(err)
		}
	}(logger)

	return logger
}
