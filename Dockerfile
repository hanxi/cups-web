FROM oven/bun AS frontend-build
WORKDIR /src/frontend
COPY frontend/package*.json ./
RUN bun install
COPY frontend ./
RUN bun run build

FROM golang:1.24 AS builder
WORKDIR /src

# copy go modules and source
COPY go.mod go.sum ./
RUN go env -w GOPROXY=https://proxy.golang.org
RUN go mod download
COPY . .
# Copy built frontend assets into expected location for go:embed
COPY --from=frontend-build /src/frontend/dist ./frontend/dist

# Build the Go binary (frontend must be built before this step in CI/local)
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags='-s -w' -o /out/cups-web ./cmd/server

FROM debian:bookworm-slim AS runtime

# Install LibreOffice (headless conversion) and minimal fonts/certificates
RUN apt-get update && apt-get install -y --no-install-recommends \
    libreoffice-core libreoffice-writer libreoffice-calc libreoffice-impress openjdk-17-jre \
    fonts-dejavu-core fonts-noto-cjk fonts-arphic-uming fonts-arphic-ukai fonts-wqy-zenhei \
    ca-certificates \
  && rm -rf /var/lib/apt/lists/*

# Create a non-root user for running the service
RUN groupadd -r nonroot && useradd -r -g nonroot nonroot

RUN mkdir -p \
    /home/nonroot/.cache/dconf \
    /home/nonroot/.config/libreoffice \
    /home/nonroot/.local/share/libreoffice \
  && chown -R nonroot:nonroot /home/nonroot/ \
  && chmod -R 755 /home/nonroot/ \
  && chmod 700 /home/nonroot/.cache/dconf

ENV DCONF_USER_CONFIG_DIR=/home/nonroot/.config/dconf
ENV HOME=/home/nonroot
ENV XDG_CACHE_HOME=/home/nonroot/.cache

COPY --from=builder /out/cups-web /cups-web
EXPOSE 8080
USER nonroot
ENTRYPOINT ["/cups-web"]
