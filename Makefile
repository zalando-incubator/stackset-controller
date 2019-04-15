.PHONY: clean test check build.local build.linux build.osx build.docker build.push

BINARY         = stackset-controller
BINARIES       = $(BINARY) traffic
LOCAL_BINARIES = $(addprefix build/,$(BINARIES))
LINUX_BINARIES = $(addprefix build/linux/,$(BINARIES))
VERSION        ?= $(shell git describe --tags --always --dirty)
IMAGE          ?= registry-write.opensource.zalan.do/teapot/$(BINARY)
E2E_IMAGE      ?= $(IMAGE)-e2e
TAG            ?= $(VERSION)
SOURCES        = $(shell find . -name '*.go')
GENERATED      = pkg/client pkg/apis/zalando.org/v1/zz_generated.deepcopy.go
GOPKGS         = $(shell go list ./... | grep -v /e2e)
BUILD_FLAGS    ?= -v
LDFLAGS        ?= -X main.version=$(VERSION) -w -s

default: build.local

clean:
	rm -rf build
	rm -rf $(GENERATED)

test: $(GENERATED)
	go test -v $(GOPKGS)

check:
	golint $(GOPKGS)
	go vet -v $(GOPKGS)

lint:
	lint:
	golangci-lint run ./...

$(GENERATED):
	./hack/update-codegen.sh

build.local: $(LOCAL_BINARIES)
build.linux: $(LINUX_BINARIES)

build/linux/e2e: $(GENERATED) $(SOURCES)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test -c -o build/linux/$(notdir $@) $(BUILD_FLAGS) ./cmd/$(notdir $@)

build/linux/%: $(GENERATED) $(SOURCES)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build $(BUILD_FLAGS) -o build/linux/$(notdir $@) -ldflags "$(LDFLAGS)" ./cmd/$(notdir $@)

build/e2e: $(GENERATED) $(SOURCES)
	CGO_ENABLED=0 go test -c -o build/$(notdir $@) ./cmd/$(notdir $@)

build/%: $(GENERATED) $(SOURCES)
	CGO_ENABLED=0 go build -o build/$(notdir $@) $(BUILD_FLAGS) -ldflags "$(LDFLAGS)" ./cmd/$(notdir $@)

build.docker: build.linux build/linux/e2e
	docker build --rm -t "$(E2E_IMAGE):$(TAG)" -f Dockerfile.e2e .
	docker build --rm -t "$(IMAGE):$(TAG)" -f Dockerfile .

build.push: build.docker
	docker push "$(E2E_IMAGE):$(TAG)"
	docker push "$(IMAGE):$(TAG)"
