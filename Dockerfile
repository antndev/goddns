FROM golang:1.23-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/goddns .

FROM scratch

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /out/goddns /usr/local/bin/goddns

USER 65532:65532

WORKDIR /app

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/goddns"]
CMD ["-config", "/config/config.yaml"]
