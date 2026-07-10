FROM golang:1.24-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -o /out/query-api ./cmd/query-api
RUN CGO_ENABLED=0 go build -o /out/gen-token ./cmd/gen-token

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/query-api ./query-api
COPY --from=build /out/gen-token ./gen-token
EXPOSE 8080
CMD ["./query-api"]
