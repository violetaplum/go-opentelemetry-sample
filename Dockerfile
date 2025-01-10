# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# 필요한 빌드 도구 설치
RUN apk add --no-cache gcc musl-dev

# Go 모듈 파일 복사 및 의존성 다운로드
COPY go.mod go.sum ./
RUN go mod download

# 소스 코드 복사 및 빌드
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o dice-app .

# Final stage
FROM alpine:latest

WORKDIR /app

# 타임존 설정을 위한 패키지 설치
RUN apk add --no-cache tzdata
ENV TZ=Asia/Seoul

# 빌드된 바이너리 복사
COPY --from=builder /app/dice-app .

EXPOSE 8080

CMD ["./dice-app"]