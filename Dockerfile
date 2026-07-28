FROM golang:1.22-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates
COPY go.mod ./
COPY . .
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/jobscout ./cmd/jobscout

FROM alpine:3.20
WORKDIR /app
RUN apk add --no-cache ca-certificates
COPY --from=build /out/jobscout /usr/local/bin/jobscout
COPY migrations ./migrations
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/jobscout"]
CMD ["serve"]
