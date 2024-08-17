VERSION --raw-output 0.8

FROM golang:latest

RUN go install github.com/go-task/task/v3/cmd/task@latest

WORKDIR /workspace

VOLUME /workspace

COPY . /workspace
