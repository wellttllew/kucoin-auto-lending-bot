FROM golang:1.15 as Builder

ADD .  /root/code 

RUN cd /root/code &&   GOFLAGS=-mod=vendor CGO_ENABLED=0 \
     GOOS=linux GOARCH=amd64  go build -v . 


# Add missing CA-certs to scratch 
# ref: https://medium.com/on-docker/use-multi-stage-builds-to-inject-ca-certs-ad1e8f01de1b
FROM alpine:latest as certs
RUN apk --update add ca-certificates



FROM scratch

COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=Builder /root/code/kucoin-auto-lending-bot  /bot 

ENTRYPOINT ["/bot"]