# syntax=docker/dockerfile:1.7

FROM node:22-alpine AS builder
WORKDIR /app

COPY frontend/package.json frontend/package-lock.json* ./
COPY frontend/scripts ./scripts
RUN if [ -f package-lock.json ]; then npm ci; else npm install; fi

COPY frontend/ ./
RUN npm run build

FROM nginx:1.29-alpine AS runtime
COPY frontend/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /app/dist /usr/share/nginx/html
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
