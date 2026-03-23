FROM golang:1.22-alpine AS builder

RUN apk add --no-cache gcc musl-dev sqlite-dev

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -ldflags "-s -w" -o /market-scanner .

FROM alpine:3.20

RUN apk add --no-cache ca-certificates sqlite-libs

COPY --from=builder /market-scanner /usr/local/bin/market-scanner

RUN mkdir -p /data
ENV FACTORY_DATA_DIR=/data
ENV DB_PATH=/data/market-scanner.db

EXPOSE 8090

ENTRYPOINT ["market-scanner"]
CMD ["serve"]
