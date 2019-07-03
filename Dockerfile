FROM golang:alpine
ENV GOPATH=/go

RUN apk update
RUN apk add build-base

ADD . /go/src/github.com/domino14/word_db_server
WORKDIR /go/src/github.com/domino14/word_db_server/cmd/searchserver

EXPOSE 8180

CMD ./searchserver
# CMD ./word_db_server -flags