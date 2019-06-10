FROM golang:alpine
ENV GOPATH=/go

RUN apk update
RUN apk add build-base

ADD . /go/src/github.com/domino14/word_db_server
WORKDIR /go/src/github.com/domino14/word_db_server

RUN go build
EXPOSE 8180

# CMD ./word_db_server -flags