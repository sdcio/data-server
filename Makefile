REMOTE_REGISTRY := registry.kmrd.dev/sdcio/data-server
TAG := $(shell git describe --tags)
IMAGE := $(REMOTE_REGISTRY):$(TAG)
TEST_IMAGE := $(IMAGE)-test
USERID := 10000

# go versions
TARGET_GO_VERSION := go1.21.4
GO_FALLBACK := go
# We prefer $TARGET_GO_VERSION if it is not available we go with whatever go we find ($GO_FALLBACK)
GO_BIN := $(shell if [ "$$(which $(TARGET_GO_VERSION))" != "" ]; then echo $$(which $(TARGET_GO_VERSION)); else echo $$(which $(GO_FALLBACK)); fi)

build:
	mkdir -p bin
	CGO_ENABLED=0 ${GO_BIN} build -o bin/datactl client/main.go 
	CGO_ENABLED=0 ${GO_BIN} build -o bin/data-server main.go

install-sdctl:
	${GO_BIN} install github.com/sdcio/sdctl@latest

robot-tests: build 
	robot tests/robot

go-tests:
	go test ./...

test: go-tests robot-tests

docker-build:
	ssh-add ./keys/id_rsa 2>/dev/null; true
	docker build --build-arg USERID=$(USERID) . -t $(IMAGE) --ssh default=$(SSH_AUTH_SOCK)

docker-push: docker-build
	docker push $(IMAGE)

release: docker-build
	docker tag $(IMAGE) $(REMOTE_REGISTRY):latest
	docker push $(REMOTE_REGISTRY):latest

docker-test:
	ssh-add ./keys/id_rsa 2>/dev/null; true
	docker build . -t $(TEST_IMAGE) -f tests/container/Dockerfile --ssh default=$(SSH_AUTH_SOCK)
	docker run -v ./tests/results:/results:rw $(TEST_IMAGE) robot --outputdir /results /app/tests/robot

run-distributed:
	./lab/distributed/run.sh build

run-combined:
	./lab/combined/run.sh build

stop:
	./lab/combined/stop.sh
	./lab/distributed/stop.sh

MOCKDIR = ./mocks
.PHONY: mocks-gen
mocks-gen: mocks-rm ## Generate mocks for all the defined interfaces.
	mkdir -p $(MOCKDIR)
	go install go.uber.org/mock/mockgen@latest
	mockgen -package=mocknetconf -source=pkg/datastore/target/netconf/driver.go -destination=$(MOCKDIR)/mocknetconf/driver.go
	mockgen -package=mockschema -source=pkg/schema/schema_client.go -destination=$(MOCKDIR)/mockschema/client.go
	mockgen -package=mockschemaclientbound -source=pkg/datastore/clients/schema/schemaClientBound.go -destination=$(MOCKDIR)/mockschemaclientbound/client.go
	mockgen -package=mockcacheclient -source=pkg/cache/cache.go -destination=$(MOCKDIR)/mockcacheclient/client.go
	mockgen -package=mocktarget -source=pkg/datastore/target/target.go -destination=$(MOCKDIR)/mocktarget/target.go
	mockgen -package=mockvalidationclient -source=pkg/datastore/clients/validationClient.go -destination=$(MOCKDIR)/mockvalidationclient/client.go

.PHONY: mocks-rm
mocks-rm: ## remove generated mocks
	rm -rf $(MOCKDIR)/*

.PHONY: unit-tests
unit-tests: mocks-gen
	rm -rf /tmp/sdcio/dataserver-tests/coverage
	mkdir -p /tmp/sdcio/dataserver-tests/coverage
	CGO_ENABLED=1 go test -cover -race ./... -v -covermode atomic -args -test.gocoverdir="/tmp/sdcio/dataserver-tests/coverage"

.PHONY: ygot
ygot:
	go install github.com/openconfig/ygot/generator@latest
	generator -output_file=tests/sdcioygot/sdcio_schema.go -package_name=sdcio_schema -generate_fakeroot -fakeroot_name=device ./tests/schema/*