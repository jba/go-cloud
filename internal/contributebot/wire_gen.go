// Code generated by Wire. DO NOT EDIT.

//go:generate wire
//+build !wireinject

package main

import (
	"cloud.google.com/go/pubsub"
	"context"
	"crypto/rsa"
	"github.com/dgrijalva/jwt-go"
	"go.opencensus.io/trace"
	"gocloud.dev/gcp"
	"gocloud.dev/health"
	"gocloud.dev/requestlog"
	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/filevar"
	"gocloud.dev/server"
	"google.golang.org/api/option"
	"net/http"
)

// Injectors from setup.go:

func inject(ctx context.Context, cfg flagConfig) (workerAndServer, func(), error) {
	projectID := projectFromConfig(cfg)
	credentials, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		return workerAndServer{}, nil, err
	}
	tokenSource := gcp.CredentialsTokenSource(credentials)
	client, cleanup, err := newPubSubClient(ctx, projectID, tokenSource)
	if err != nil {
		return workerAndServer{}, nil, err
	}
	subscription := subscriptionFromConfig(client, cfg)
	roundTripper := _wireRoundTripperValue
	mainGitHubAppAuth, cleanup2, err := gitHubAppAuthFromConfig(roundTripper, cfg)
	if err != nil {
		cleanup()
		return workerAndServer{}, nil, err
	}
	mainWorker := newWorker(subscription, mainGitHubAppAuth)
	logger := _wireLoggerValue
	v := healthChecks(mainWorker)
	exporter := _wireExporterValue
	sampler := trace.NeverSample()
	defaultDriver := _wireDefaultDriverValue
	options := &server.Options{
		RequestLogger:         logger,
		HealthChecks:          v,
		TraceExporter:         exporter,
		DefaultSamplingPolicy: sampler,
		Driver:                defaultDriver,
	}
	serverServer := server.New(options)
	mainWorkerAndServer := workerAndServer{
		worker: mainWorker,
		server: serverServer,
	}
	return mainWorkerAndServer, func() {
		cleanup2()
		cleanup()
	}, nil
}

var (
	_wireRoundTripperValue  = http.DefaultTransport
	_wireLoggerValue        = (requestlog.Logger)(nil)
	_wireExporterValue      = (trace.Exporter)(nil)
	_wireDefaultDriverValue = &server.DefaultDriver{}
)

// setup.go:

func setup(ctx context.Context, cfg flagConfig) (*worker, *server.Server, func(), error) {
	ws, cleanup, err := inject(ctx, cfg)
	if err != nil {
		return nil, nil, nil, err
	}
	return ws.worker, ws.server, cleanup, nil
}

type workerAndServer struct {
	worker *worker
	server *server.Server
}

func gitHubAppAuthFromConfig(rt http.RoundTripper, cfg flagConfig) (*gitHubAppAuth, func(), error) {
	d := runtimevar.NewDecoder(new(rsa.PrivateKey), func(p []byte, val interface{}) error {
		key, err := jwt.ParseRSAPrivateKeyFromPEM(p)
		if err != nil {
			return err
		}
		*(val.(**rsa.PrivateKey)) = key
		return nil
	})
	v, err := filevar.New(cfg.keyPath, d, nil)
	if err != nil {
		return nil, nil, err
	}
	auth := newGitHubAppAuth(cfg.gitHubAppID, v, rt)
	return auth, func() {
		auth.Stop()
		v.Close()
	}, nil
}

func newPubSubClient(ctx context.Context, id gcp.ProjectID, ts gcp.TokenSource) (*pubsub.Client, func(), error) {
	c, err := pubsub.NewClient(ctx, string(id), option.WithTokenSource(ts))
	if err != nil {
		return nil, nil, err
	}
	return c, func() { c.Close() }, nil
}

func subscriptionFromConfig(client *pubsub.Client, cfg flagConfig) *pubsub.Subscription {
	return client.SubscriptionInProject(cfg.subscription, cfg.project)
}

func projectFromConfig(cfg flagConfig) gcp.ProjectID {
	return gcp.ProjectID(cfg.project)
}

func healthChecks(w *worker) []health.Checker {
	return []health.Checker{w}
}
