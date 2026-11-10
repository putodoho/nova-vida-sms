# Build the API and the admin CLI.
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download || true
COPY . .
# go.sum is generated on first build if it is not committed yet.
RUN go mod tidy \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/cli ./cmd/cli

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/server /out/cli /app/
WORKDIR /app
EXPOSE 8080
USER nonroot
ENTRYPOINT ["/app/server"]
