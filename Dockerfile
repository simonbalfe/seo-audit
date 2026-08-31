FROM golang:1.25-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
RUN CGO_ENABLED=0 go build -o /seoaudit ./cmd/seoaudit

FROM alpine:3.22
RUN apk add --no-cache ca-certificates chromium
WORKDIR /app
COPY --from=build /seoaudit /usr/local/bin/seoaudit
EXPOSE 8090
CMD ["seoaudit", "serve"]
