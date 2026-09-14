FROM golang

ENV CGO_ENABLED=0
ENV GO111MODULE=on

WORKDIR /go/src/logstorm

COPY go.mod go.sum ./
RUN go mod download

COPY . ./
RUN go build -o /bin/logstorm

FROM scratch
COPY --from=0 /bin/logstorm /bin/logstorm
ENTRYPOINT ["logstorm"]
