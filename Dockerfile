FROM golang:alpine
ENV GOPATH=/go

RUN apk update
RUN apk add build-base

ADD . /go/src/github.com/domino14/word_db_maker
WORKDIR /go/src/github.com/domino14/word_db_maker

RUN go build

# CMD ./word_db_maker -flags