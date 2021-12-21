FROM golang:alpine as base

RUN apk update && apk add --no-cache git ca-certificates && update-ca-certificates
ENV USER=capco
ENV UID=1001 
RUN adduser \    
    --disabled-password \    
    --gecos "" \    
    --shell /sbin/nologin \    
    --no-create-home \    
    --uid $UID \    
    $USER

ADD main.go src/
ADD go.* src/
ADD argo/ src/argo/
ADD aws/ src/aws/
ADD github/ src/github/
ADD slack/ src/slack/
ADD util/ src/util/
RUN cd src && go mod tidy && go mod verify && CGO_ENABLED=0 go build -o /go/bin/deploy-bot

FROM scratch
COPY --from=base /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=base /etc/passwd /etc/passwd
COPY --from=base /etc/group /etc/group
COPY --from=base /go/bin/deploy-bot /go/bin/deploy-bot
EXPOSE 4040
USER capco:capco
CMD ["/go/bin/deploy-bot"]
