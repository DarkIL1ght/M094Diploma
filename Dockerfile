# B-02. Образ сервера. Сборка на платформе runner'а, под целевую архитектуру (в том числе arm64 для Oracle Cloud):
#   docker build -t m094 .
#   docker buildx build --platform linux/arm64 -t m094 .
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY migrations ./migrations
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# В итоговом образе нет оболочки и пакетов, только бинарник; запуск не от root.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/server /server
# seed для `server seed` в выкатке (S-05): путь по умолчанию content/seed — от рабочего каталога /
COPY content/seed /content/seed
WORKDIR /
ENV ADDR=:8080
EXPOSE 8080
# healthcheck — подкоманда самого сервера: curl в образе нет
HEALTHCHECK --interval=10s --timeout=4s --start-period=5s --retries=3 CMD ["/server", "healthcheck"]
ENTRYPOINT ["/server"]
