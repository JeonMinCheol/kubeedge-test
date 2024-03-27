# Golang 버전 설정
FROM golang:1.19-alpine

# 작업 디렉토리 설정
WORKDIR /app

# Golang 애플리케이션 소스 코드 복사
COPY . .

# Golang 애플리케이션 빌드
RUN go build -o main

# 실행 명령 설정
ENTRYPOINT  ["./main", "-logtostderr=true"]