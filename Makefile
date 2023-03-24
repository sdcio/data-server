
BRANCH :=$(shell git rev-parse --abbrev-ref HEAD)
COMMIT := $(shell git rev-parse --short HEAD)
REMOTE_REGISTRY :=registry.kmrd.dev/iptecharch/schema-server
TAG := 0.0.0-$(BRANCH)-$(COMMIT)
IMAGE := $(REMOTE_REGISTRY):$(TAG)

build:
	mkdir -p bin
	go1.19.5 build -o bin/client client/main.go 
	go1.19.5 build -o bin/server main.go
	go1.19.5 build -o bin/bulk tests/bulk/main.go

docker-build:
	docker build . -t $(IMAGE)

docker-push: docker-build
	docker tag $(IMAGE) $(REMOTE_REGISTRY):latest
	docker push $(IMAGE)

run-distributed:
	./lab/distributed/run.sh build

run-combined:
	./lab/combined/run.sh build

stop:
	./lab/combined/stop.sh
	./lab/distributed/stop.sh