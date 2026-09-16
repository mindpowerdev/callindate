FROM golang:1.25-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/bot ./cmd/bot

FROM alpine:3.20
# ca-certificates нужен для TLS при обращении к api.telegram.org.
# tzdata не нужен — база часовых поясов зашита в бинарник через time/tzdata.
RUN apk add --no-cache ca-certificates

COPY --from=build /out/bot /bot

ENTRYPOINT ["/bot"]
