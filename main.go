// Copyright (c) 2022 AccelByte Inc. All Rights Reserved.
// This is licensed software from AccelByte Inc, for limitations
// and restrictions contact your company contract manager.

package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/AccelByte/accelbyte-go-sdk/services-api/pkg/factory"
	"github.com/AccelByte/accelbyte-go-sdk/services-api/pkg/repository"
	"github.com/AccelByte/accelbyte-go-sdk/services-api/pkg/service/iam"
	promgrpc "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/propagators/b3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"log/slog"

	"lootbox-roll-function-grpc-plugin-server-go/pkg/common"
	pb "lootbox-roll-function-grpc-plugin-server-go/pkg/pb"
	"lootbox-roll-function-grpc-plugin-server-go/pkg/service"

	sdkAuth "github.com/AccelByte/accelbyte-go-sdk/services-api/pkg/utils/auth"
	prometheusCollectors "github.com/prometheus/client_golang/prometheus/collectors"
)

const (
	id              = int64(1)
	environment     = "production"
	metricsEndpoint = "/metrics"
	metricsPort     = 8080
	grpcPort        = 6565
)

var (
	serviceName = "extend-app-lootbox-roll"
	logLevelStr = common.GetEnv("LOG_LEVEL", "info")
)

// parseLogLevel converts a log level string to slog.Level
func parseLogLevel(levelStr string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(levelStr)) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// requireEnv exits when any of the named environment variables is empty, naming all of the missing
// ones at once so that a misconfigured deployment does not have to be fixed one restart at a time.
func requireEnv(logger *slog.Logger, keys ...string) {
	missing := make([]string, 0, len(keys))
	for _, key := range keys {
		if common.GetEnv(key, "") == "" {
			missing = append(missing, key)
		}
	}

	if len(missing) > 0 {
		logger.Error("missing required environment variables", "missing", strings.Join(missing, ", "))
		os.Exit(1)
	}
}

func main() {
	go func() {
		runtime.SetBlockProfileRate(1)
		runtime.SetMutexProfileFraction(10)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logLevel := parseLogLevel(logLevelStr)
	opts := &slog.HandlerOptions{
		Level: logLevel,
	}
	handler := slog.NewJSONHandler(os.Stdout, opts)
	logger := slog.New(handler)

	logger.Info("starting app server..")

	// Echo the AGS environment this app talks to. Pointing at the wrong environment yields tokens
	// signed by keys this app never fetched, and without this line that cause is invisible in logs.
	logger.Info("accelbyte environment",
		"AB_BASE_URL", common.GetEnv("AB_BASE_URL", "(unset)"),
		"AB_NAMESPACE", common.GetEnv("AB_NAMESPACE", "(unset)"),
	)

	loggingOptions := []logging.Option{
		logging.WithLogOnEvents(logging.PayloadSent),
		logging.WithFieldsFromContext(func(ctx context.Context) logging.Fields {
			if span := trace.SpanContextFromContext(ctx); span.IsSampled() {
				return logging.Fields{"traceID", span.TraceID().String()}
			}

			return nil
		}),
		logging.WithLevels(logging.DefaultClientCodeToLevel),
		logging.WithDurationField(logging.DurationToDurationField),
	}

	srvMetrics := promgrpc.NewServerMetrics()
	unaryServerInterceptors := []grpc.UnaryServerInterceptor{
		srvMetrics.UnaryServerInterceptor(),
		logging.UnaryServerInterceptor(common.InterceptorLogger(logger), loggingOptions...),
	}
	streamServerInterceptors := []grpc.StreamServerInterceptor{
		srvMetrics.StreamServerInterceptor(),
		logging.StreamServerInterceptor(common.InterceptorLogger(logger), loggingOptions...),
	}

	// Set Tracer Provider
	if val := common.GetEnv("OTEL_SERVICE_NAME", ""); val != "" {
		serviceName = "extend-app-lr-" + strings.ToLower(val)
	}
	tracerProvider, err := common.NewTracerProvider(serviceName, environment, id)
	if err != nil {
		logger.Error("failed to create tracer provider", "error", err)
		os.Exit(1)
	}
	otel.SetTracerProvider(tracerProvider)
	defer func(ctx context.Context) {
		if err := tracerProvider.Shutdown(ctx); err != nil {
			logger.Error("failed to shutdown tracer provider", "error", err)
			os.Exit(1)
		}
	}(ctx)
	logger.Info("set tracer provider", "name", serviceName, "environment", environment, "id", id)

	// Set Text Map Propagator
	b := b3.New(b3.WithInjectEncoding(b3.B3MultipleHeader))
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			b,
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	// Set tracing HTTP transport globally so all outgoing HTTP calls are traced
	http.DefaultTransport = common.NewTracingRoundTripper()

	// Preparing the IAM authorization
	var tokenRepo repository.TokenRepository = sdkAuth.DefaultTokenRepositoryImpl()
	var configRepo repository.ConfigRepository = sdkAuth.DefaultConfigRepositoryImpl()
	var refreshRepo repository.RefreshTokenRepository = &sdkAuth.RefreshTokenImpl{RefreshRate: 0.8, AutoRefresh: true}

	oauthService := iam.OAuth20Service{
		Client:                 factory.NewIamClient(configRepo),
		TokenRepository:        tokenRepo,
		RefreshTokenRepository: refreshRepo,
		ConfigRepository:       configRepo,
	}

	// The validator is constructed here because the interceptors must be wired before the gRPC
	// server is created, but it is initialized further below -- after the server is already
	// listening -- so that health probes stay answerable while it retries.
	authEnabled := strings.ToLower(common.GetEnv("PLUGIN_GRPC_SERVER_AUTH_ENABLED", "true")) == "true"
	if authEnabled {
		requireEnv(logger, "AB_BASE_URL", "AB_CLIENT_ID", "AB_CLIENT_SECRET", "AB_NAMESPACE")

		refreshInterval := common.GetEnvInt("REFRESH_INTERVAL", 600)
		common.Validator = common.NewTokenValidator(oauthService, time.Duration(refreshInterval)*time.Second, true)

		unaryServerInterceptors = append(unaryServerInterceptors, common.UnaryAuthServerIntercept)
		streamServerInterceptors = append(streamServerInterceptors, common.StreamAuthServerIntercept)
		logger.Info("added auth interceptors")
	} else {
		logger.Warn("gRPC server auth is disabled, every caller is trusted",
			"env", "PLUGIN_GRPC_SERVER_AUTH_ENABLED=false")
	}

	// Create gRPC Server
	grpcServer := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(unaryServerInterceptors...),
		grpc.ChainStreamInterceptor(streamServerInterceptors...),
	)

	// Register Filter Service
	lootBoxServiceServer := service.NewLootBoxServiceServer()
	pb.RegisterLootBoxServer(grpcServer, lootBoxServiceServer)

	// Enable gRPC Reflection
	reflection.Register(grpcServer)
	logger.Info("gRPC reflection enabled")

	// Enable gRPC Health Check. health.NewServer seeds the overall status as SERVING, so both
	// statuses are reset immediately: until the token validator holds the IAM public keys every RPC
	// is rejected, and reporting SERVING through that would keep a broken pod in rotation.
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	healthServer.SetServingStatus(pb.LootBox_ServiceDesc.ServiceName, grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)

	// Register Prometheus Metrics
	srvMetrics.InitializeMetrics(grpcServer)
	prometheusRegistry := prometheus.NewRegistry()
	prometheusRegistry.MustRegister(
		prometheusCollectors.NewGoCollector(),
		prometheusCollectors.NewProcessCollector(prometheusCollectors.ProcessCollectorOpts{}),
		srvMetrics,
	)

	go func() {
		http.Handle(metricsEndpoint, promhttp.HandlerFor(prometheusRegistry, promhttp.HandlerOpts{}))
		log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", metricsPort), nil))
	}()
	logger.Info("serving prometheus metrics", "port", metricsPort, "endpoint", metricsEndpoint)

	logger.Info("set text map propagator")

	// Start gRPC Server
	logger.Info("starting gRPC server..")
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", grpcPort))
	if err != nil {
		logger.Error("failed to listen to tcp", "port", grpcPort, "error", err)
		os.Exit(1)
	}
	go func() {
		if err = grpcServer.Serve(lis); err != nil {
			logger.Error("failed to run gRPC server", "error", err)
			os.Exit(1)
		}
	}()
	logger.Info("gRPC server started")

	markServing := func() {
		healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
		healthServer.SetServingStatus(pb.LootBox_ServiceDesc.ServiceName, grpc_health_v1.HealthCheckResponse_SERVING)
		logger.Info("health status set to SERVING")
	}

	if authEnabled {
		// Retry in the background: a failed Initialize leaves the validator with no public keys and
		// nothing scheduled to refetch them, so without this a transient IAM failure at startup
		// would reject every token for the lifetime of the process.
		go func(ctx context.Context) {
			common.InitializeValidatorWithRetry(ctx, common.Validator, logger)
			if common.IsValidatorReady() {
				markServing()
			}
		}(ctx)
	} else {
		common.SetValidatorReady(true)
		markServing()
	}

	logger.Info("app server started")

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	logger.Info("signal received")
}
