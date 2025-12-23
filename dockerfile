FROM cgr.dev/chainguard/go:latest AS build-env

COPY ./ /app
WORKDIR /app

RUN go build -o /app/main

FROM cgr.dev/chainguard/bash:latest
COPY --from=build-env /app/main /app

WORKDIR /home/nonroot
USER nonroot