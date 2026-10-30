FROM node:24.22.2-alpine AS frontend
WORKDIR /build
RUN npm install --global npm@12.2.1 --ignore-scripts
COPY package*.json .npmrc ./
RUN npm ci
COPY .config .config
COPY tsconfig.json eslint.config.mjs .prettierrc.js ./
COPY src src
COPY README.md CHANGELOG.md LICENSE ./
RUN npm run build

FROM golang:1.27.3-alpine AS backend
WORKDIR /build
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download && go install github.com/magefile/mage@v1.17.2
COPY Magefile.go package.json ./
COPY pkg pkg
COPY src/plugin.json src/plugin.json
RUN mage -v build:linux

FROM grafana/grafana:13.3.0
COPY --from=frontend /build/dist /var/lib/grafana/plugins/mrqs001-cloudflareanalytics-datasource
COPY --from=backend /build/dist /var/lib/grafana/plugins/mrqs001-cloudflareanalytics-datasource
