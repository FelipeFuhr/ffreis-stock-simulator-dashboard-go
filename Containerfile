# scan-fix(ci:blank-build-arg): both ARGs must be declared before the FIRST
# FROM. Docker only resolves a FROM's base image against ARGs declared in
# that pre-FROM global scope — an ARG declared between stages (as
# RUNTIME_IMAGE previously was, right before its own FROM) is scoped to the
# preceding stage's instructions, not to a later FROM's image reference, so
# it resolved blank ("base name (${RUNTIME_IMAGE}) should not be blank").
# Confirmed locally against both BuildKit-equivalent and buildah/podman
# builders: a minimal 2-stage reproduction with the ARG between stages fails
# identically; moving both ARGs above the first FROM fixes it in both.
ARG BUILDER_IMAGE=golang:1.25.8-alpine
ARG RUNTIME_IMAGE=gcr.io/distroless/static-debian12:nonroot

FROM ${BUILDER_IMAGE} AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags='-s -w' -o /out/dashboard ./cmd/dashboard

FROM ${RUNTIME_IMAGE}

ENV DASHBOARD_PORT=8080
EXPOSE 8080

COPY --from=build /out/dashboard /dashboard

ENTRYPOINT ["/dashboard"]
