FROM m.daocloud.io/docker.io/library/node:22-alpine AS frontend-builder
ARG VITE_API_BASE_URL=
ENV VITE_API_BASE_URL=$VITE_API_BASE_URL
WORKDIR /frontend
COPY frontend/package.json frontend/package-lock.json* ./
COPY frontend/scripts ./scripts
RUN if [ -f package-lock.json ]; then npm ci; else npm install; fi
COPY frontend/ ./
RUN npm run build

FROM m.daocloud.io/docker.io/library/golang:1.25 AS backend-builder
WORKDIR /app
RUN go env -w GOPROXY=https://goproxy.cn,direct
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/gos-server ./cmd/server

FROM m.daocloud.io/docker.io/library/rockylinux:9-minimal AS runtime
WORKDIR /app
ENV PIP_ROOT_USER_ACTION=ignore

RUN microdnf install -y ca-certificates tzdata curl git openssh-clients nginx python3 python3-pip \
  && python3 -m pip install --no-cache-dir supervisor \
  && microdnf clean all \
  && rm -rf /var/cache/yum /var/cache/dnf \
  && rm -f /etc/nginx/conf.d/default.conf /etc/nginx/default.d/*

COPY --from=backend-builder /out/gos-server /usr/local/bin/gos-server
COPY --from=frontend-builder /frontend/dist /usr/share/nginx/html
COPY docker/nginx.conf /etc/nginx/conf.d/gos.conf
COPY docker/supervisord.conf /etc/supervisord.conf
COPY docker/entrypoint.sh /usr/local/bin/gos-entrypoint
COPY configs/config.container.template.json /app/configs/config.container.template.json

RUN sed -i 's/\r$//' /usr/local/bin/gos-entrypoint \
  && chmod +x /usr/local/bin/gos-entrypoint

EXPOSE 5174 8081
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 CMD curl -fsS http://127.0.0.1:8081/healthz >/dev/null && curl -fsS -H 'Accept: text/html' http://127.0.0.1:5174/ >/dev/null || exit 1
ENTRYPOINT ["/usr/local/bin/gos-entrypoint"]
CMD ["/usr/local/bin/supervisord", "-c", "/etc/supervisord.conf"]
