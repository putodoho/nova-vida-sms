# Build the API and the admin CLI.
# Requires a committed go.sum (run `go mod tidy` in api/ once and commit it).
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/cli ./cmd/cli

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/server /out/cli /app/
WORKDIR /app
EXPOSE 8080
USER nonroot
ENTRYPOINT ["/app/server"]
