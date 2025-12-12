# build not working right now
FROM golang:1.25.3 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/gatelite ./cmd/gatelite

# run
FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=build /out/gatelite /app/gatelite
COPY configs/config.yaml /app/config.yaml
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/gatelite"]