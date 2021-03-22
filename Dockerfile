FROM golang:1.15 as Builder

ADD .  /root/code 

RUN cd /root/code &&   GOFLAGS=-mod=vendor CGO_ENABLED=0 \
     GOOS=linux GOARCH=amd64  go build -v . 


FROM scratch

COPY --from=Builder /root/code/kucoin-auto-lending-bot  /bot 

ENTRYPOINT ["/bot"]