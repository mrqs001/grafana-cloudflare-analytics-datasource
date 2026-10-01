FROM node:24.14.0-alpine AS frontend
WORKDIR /build
COPY package*.json .npmrc ./
RUN npm ci
COPY .config .config
COPY tsconfig.json eslint.config.mjs .prettierrc.js ./
COPY src src
COPY README.md CHANGELOG.md LICENSE ./
RUN npm run build

FROM golang:1.26.5-alpine AS backend
WORKDIR /build
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download && go install github.com/magefile/mage@v1.15.0
COPY Magefile.go ./
COPY pkg pkg
COPY src/plugin.json src/plugin.json
RUN mage -v build:linux

FROM grafana/grafana:13.2.3
COPY --from=frontend /build/dist /var/lib/grafana/plugins/mrqs001-cloudflareanalytics-datasource
COPY --from=backend /build/dist /var/lib/grafana/plugins/mrqs001-cloudflareanalytics-datasource
