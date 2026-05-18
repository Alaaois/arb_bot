FROM golang:1.25-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/arb-bot ./cmd/arb-bot

FROM alpine:3.22

RUN addgroup -S app && adduser -S app -G app

WORKDIR /app

COPY --from=build /out/arb-bot /app/arb-bot
COPY configs ./configs

USER app

EXPOSE 8080

ENTRYPOINT ["/app/arb-bot"]
CMD ["-config", "configs/local.yaml"]
