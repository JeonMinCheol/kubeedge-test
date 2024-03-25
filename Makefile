.PHONY: default build
build:
	go build main.go
	docker build -t jmc0504/kubeedge_test:v1.1 .