FROM golang:1.17.3 as base
RUN adduser --disabled-password deploy-bot
USER deploy-bot
ADD main.go src/
ADD util/ src/util/
ADD go.* src/
#RUN set -x && \
#    cd src && go mod init && go get && go env -w GO111MODULE=off && \
#    CGO_ENABLED=0 go build -o /go/bin/deploy-bot
RUN set -x && \
    cd src && go get && \
    CGO_ENABLED=0 go build -o /go/bin/deploy-bot

# final stage
FROM scratch
#WORKDIR /app
COPY --from=base /go/bin/deploy-bot /go/bin/deploy-bot
EXPOSE 4040
ENTRYPOINT ["/go/bin/deploy-bot"]
