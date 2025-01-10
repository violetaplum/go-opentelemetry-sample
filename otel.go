package main

import (
	"context"
	"errors"
	"go.opentelemetry.io/otel/exporters/prometheus"
	llog "log"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace"
)

// setupOTelSDK는 OpenTelemetry 파이프라인을 부트스트랩합니다.
// 에러가 반환되지 않으면, 적절한 정리를 위해 shutdown을 호출하세요.
func setupOTelSDK(ctx context.Context) (shutdown func(context.Context) error, err error) {
	var shutdownFuncs []func(context.Context) error

	// shutdown은 shutdownFuncs를 통해 등록된 정리 함수들을 호출합니다.
	// 호출에서 발생한 에러들은 결합됩니다.
	// 등록된 각 정리 함수는 한 번만 호출됩니다.
	shutdown = func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		shutdownFuncs = nil
		return err
	}

	// handleErr는 정리를 위해 shutdown을 호출하고 모든 에러가 반환되도록 합니다.
	handleErr := func(inErr error) {
		err = errors.Join(inErr, shutdown(ctx))
	}

	// Propagator 설정
	prop := newPropagator()
	otel.SetTextMapPropagator(prop)

	// 추적 제공자 설정
	tracerProvider, err := newTraceProvider()
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)

	// 측정 제공자 설정
	meterProvider, err := newMeterProvider()
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, meterProvider.Shutdown)
	otel.SetMeterProvider(meterProvider)

	promMeterProvider, err := newPrometheusMeterProvider()
	if err != nil {
		handleErr(err)
		return
	}
	// Prometheus provider 도 전역 provider 로 설정
	shutdownFuncs = append(shutdownFuncs, promMeterProvider.Shutdown) // 없어야하나? 있어야하나?
	otel.SetMeterProvider(promMeterProvider)

	// 로거 제공자 설정
	loggerProvider, err := newLoggerProvider()
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, loggerProvider.Shutdown)
	global.SetLoggerProvider(loggerProvider)

	return
}

func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

func newTraceProvider() (*trace.TracerProvider, error) {
	traceExporter, err := stdouttrace.New(
		stdouttrace.WithPrettyPrint())
	if err != nil {
		return nil, err
	}

	traceProvider := trace.NewTracerProvider(
		trace.WithBatcher(traceExporter,
			// 기본값은 5초입니다. 시연을 위해 1초로 설정했습니다.
			trace.WithBatchTimeout(time.Second)),
	)
	return traceProvider, nil
}

func newMeterProvider() (*metric.MeterProvider, error) {
	metricExporter, err := stdoutmetric.New()
	if err != nil {
		return nil, err
	}

	meterProvider := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(metricExporter,
			// 기본값은 1분입니다. 시연을 위해 3초로 설정했습니다.
			metric.WithInterval(3*time.Second))),
	)
	return meterProvider, nil
}

func newLoggerProvider() (*log.LoggerProvider, error) {
	logExporter, err := stdoutlog.New()
	if err != nil {
		return nil, err
	}

	loggerProvider := log.NewLoggerProvider(
		log.WithProcessor(log.NewBatchProcessor(logExporter)),
	)
	return loggerProvider, nil
}

func newPrometheusMeterProvider() (*metric.MeterProvider, error) {
	exporter, err := prometheus.New(
		prometheus.WithoutTargetInfo(),
		prometheus.WithoutScopeInfo(),
		// 디버깅 테스트용
		prometheus.WithNamespace("dice_game"), // 네임스페이스 추가
	)
	if err != nil {
		llog.Printf("Prometheus exporter creation failed: %v", err)
		return nil, err
	}

	meterProvider := metric.NewMeterProvider(
		metric.WithReader(exporter))

	// 초기화 후 메트릭이 제대로 등록되었는지 확인하기 위한 로그
	llog.Printf("Prometheus meter provider initialized")

	return meterProvider, nil
}
