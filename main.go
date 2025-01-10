package main

import (
	"context"
	"errors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func main() {
	if err := run(); err != nil {
		log.Fatalln(err)
	}
}

func run() (err error) {
	// SIGINT(CTRL+C)를 정상적으로 처리합니다.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// OpenTelemetry 설정
	otelShutdown, err := setupOTelSDK(ctx)
	if err != nil {
		return
	}
	// 메모리 누수 방지를 위해 종료를 적절히 처리합니다.
	defer func() {
		err = errors.Join(err, otelShutdown(context.Background()))
	}()

	// HTTP 서버 시작
	srv := &http.Server{
		Addr:         ":8080",
		BaseContext:  func(_ net.Listener) context.Context { return ctx },
		ReadTimeout:  time.Second,
		WriteTimeout: 10 * time.Second,
		Handler:      newHTTPHandler(),
	}
	srvErr := make(chan error, 1)
	go func() {
		srvErr <- srv.ListenAndServe()
	}()

	// 인터럽트 대기
	select {
	case err = <-srvErr:
		// HTTP 서버 시작 시 오류 발생
		return
	case <-ctx.Done():
		// 첫 번째 CTRL+C 대기
		// 최대한 빨리 시그널 알림 수신을 중지합니다.
		stop()
	}

	// Shutdown이 호출되면 ListenAndServe는 즉시 ErrServerClosed를 반환합니다.
	err = srv.Shutdown(context.Background())
	return
}

func newHTTPHandler() http.Handler {
	mux := http.NewServeMux()

	// handleFunc는 mux.HandleFunc의 대체 함수로
	// 핸들러의 HTTP 계측을 http.route로 보강합니다.
	handleFunc := func(pattern string, handlerFunc func(http.ResponseWriter, *http.Request)) {
		// HTTP 계측을 위한 "http.route" 구성
		handler := otelhttp.WithRouteTag(pattern, http.HandlerFunc(handlerFunc))
		mux.Handle(pattern, handler)
	}

	// 핸들러 등록
	handleFunc("/rolldice/", rolldice)
	handleFunc("/rolldice/{player}", rolldice)

	// Prometheus metrics 엔드포인트 추가
	mux.Handle("/metrics", promhttp.Handler())

	// 전체 서버에 대한 HTTP 계측 추가
	handler := otelhttp.NewHandler(mux, "/")
	return handler
}
