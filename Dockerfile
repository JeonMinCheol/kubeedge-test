FROM ubuntu:latest

CMD mkdir -p build

COPY . build/

WORKDIR build

ENTRYPOINT ["/build/main","-logtostderr=true"]