FROM golang:1.22-alpine AS build
WORKDIR /app
COPY go.mod ./
RUN go mod download || true
COPY . .
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -o /stockflow ./cmd/server

FROM alpine:3.20
WORKDIR /app
COPY --from=build /stockflow /app/stockflow
EXPOSE 8080
ENV PORT=8080
CMD ["/app/stockflow"]
