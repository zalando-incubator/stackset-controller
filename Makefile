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
CRD_SOURCES    = $(shell find pkg/apis/zalando.org -name '*.go')
GENERATED_CRDS = docs/stackset_crd.yaml docs/stack_crd.yaml
GENERATED      = pkg/apis/zalando.org/v1/zz_generated.deepcopy.go
CONTROLLER_GEN = ./build/controller-gen
GOPKGS         = $(shell go list ./... | grep -v /e2e | grep -v vendor)
BUILD_FLAGS    ?= -v
LDFLAGS        ?= -X main.version=$(VERSION) -w -s

default: build.local

clean:
	rm -rf build
	rm -rf $(GENERATED)
	rm -f $(GENERATED_CRDS)

test: $(GENERATED)
	go test -v $(GOPKGS)

check: $(GENERATED)
	go mod download
	golangci-lint run --timeout=2m ./...

$(GENERATED):
	./hack/update-codegen.sh

$(CONTROLLER_GEN):
	mkdir -p build
	GOBIN=$(shell pwd)/build go install sigs.k8s.io/controller-tools/cmd/controller-gen

crds: $(GENERATED_CRDS)

$(GENERATED_CRDS): $(CONTROLLER_GEN) $(GENERATED) $(CRD_SOURCES)
	$(CONTROLLER_GEN) crd:preserveUnknownFields=false paths=./pkg/apis/... output:crd:dir=docs || /bin/true || true
	mv docs/zalando.org_stacksets.yaml docs/stackset_crd.yaml
	mv docs/zalando.org_stacks.yaml docs/stack_crd.yaml
	# workaround for CRD issue with k8s 1.18 & controller-gen 0.3
	# ref: https://github.com/kubernetes/kubernetes/issues/91395
	perl -i -p0e 's/\s*x-kubernetes-list-map-keys:.*?x-kubernetes-list-type: map//seg' $(GENERATED_CRDS)

build.local: $(LOCAL_BINARIES)
build.linux: $(LINUX_BINARIES)

build/linux/e2e: $(GENERATED) $(SOURCES)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test -c -o build/linux/$(notdir $@) $(BUILD_FLAGS) ./cmd/$(notdir $@)

build/darwin/e2e: $(GENERATED) $(SOURCES)
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go test -c -o build/darwin/$(notdir $@) $(BUILD_FLAGS) ./cmd/$(notdir $@)

build/linux/%: $(GENERATED) $(SOURCES)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build $(BUILD_FLAGS) -o build/linux/$(notdir $@) -ldflags "$(LDFLAGS)" ./cmd/$(notdir $@)

build/darwin/%: $(GENERATED) $(SOURCES)
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build $(BUILD_FLAGS) -o build/darwin/$(notdir $@) -ldflags "$(LDFLAGS)" ./cmd/$(notdir $@)

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
