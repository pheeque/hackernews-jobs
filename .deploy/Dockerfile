##
## Build
##
FROM golang:1.16-buster AS build

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY *.go ./

RUN go build -o /hackernews-jobs

##
## Deoploy
##
FROM gcr.io/distroless/base-debian10

WORKDIR /

COPY --from=build /hackernews-jobs /hackernews-jobs

ARG BOT_MAIL_HOST=${BOT_MAIL_HOST}
ARG BOT_MAIL_PORT=${BOT_MAIL_PORT}
ARG BOT_MAIL_FROM=${BOT_MAIL_FROM}
ARG BOT_MAIL_TO=${BOT_MAIL_TO}
ARG BOT_MAIL_USERNAME=${BOT_MAIL_USERNAME}
ARG BOT_MAIL_PASSWORD=${BOT_MAIL_PASSWORD}
ARG BOT_HEALTHCHECKS_ENDPOINT=${BOT_HEALTHCHECKS_ENDPOINT}

ENTRYPOINT ["/hackernews-jobs"]