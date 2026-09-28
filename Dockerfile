# syntax=docker/dockerfile:1

FROM golang:1.23-bookworm AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/support-api ./cmd/support-api
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/support-worker ./cmd/support-worker

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/support-api /app/support-api
COPY --from=build /out/support-worker /app/support-worker
COPY migrations /app/migrations
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app/support-api"]
