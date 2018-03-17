FROM golang:alpine
ENV GOPATH=/go
ADD . /go/src/github.com/domino14/word_db_maker
WORKDIR /go/src/github.com/domino14/word_db_maker

RUN apk update
RUN apk add build-base
RUN go build

# CMD ./word_db_maker -flags