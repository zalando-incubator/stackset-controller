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
CRD_TYPE_SOURCE = pkg/apis/zalando.org/v1/types.go
GENERATED_CRDS = docs/stackset_crd.yaml docs/stack_crd.yaml
GENERATED      = pkg/apis/zalando.org/v1/zz_generated.deepcopy.go
GOPKGS         = $(shell go list ./... | grep -v /e2e)
BUILD_FLAGS    ?= -v
LDFLAGS        ?= -X main.version=$(VERSION) -w -s

default: build.local

clean:
	rm -rf build
	rm -rf $(GENERATED)
	rm -f $(GENERATED_CRDS)

test: $(GENERATED)
	go test -v -coverprofile=profile.cov $(GOPKGS)

check: $(GENERATED)
	go mod download
	golangci-lint run --timeout=2m ./...

$(GENERATED): go.mod $(CRD_TYPE_SOURCE)
	./hack/update-codegen.sh

$(GENERATED_CRDS): $(GENERATED) $(CRD_SOURCES)
	go run sigs.k8s.io/controller-tools/cmd/controller-gen crd:crdVersions=v1,allowDangerousTypes=true paths=./pkg/apis/... output:crd:dir=docs
	go run hack/crd/trim.go < docs/zalando.org_stacksets.yaml > docs/stackset_crd.yaml
	go run hack/crd/trim.go < docs/zalando.org_stacks.yaml > docs/stack_crd.yaml
	rm docs/zalando.org_stacksets.yaml docs/zalando.org_stacks.yaml
	rm -f docs/zalando.org_platformcredentialssets.yaml

build.local: $(LOCAL_BINARIES) $(GENERATED_CRDS)
build.linux: $(LINUX_BINARIES)
build.linux.amd64: build/linux/amd64/$(BINARY)
build.linux.arm64: build/linux/arm64/$(BINARY)

build/linux/e2e: $(GENERATED) $(SOURCES)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test -c -o build/linux/$(notdir $@) $(BUILD_FLAGS) ./cmd/$(notdir $@)

build/linux/amd64/e2e: $(GENERATED) $(SOURCES)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test -c -o build/linux/amd64/$(notdir $@) $(BUILD_FLAGS) ./cmd/$(notdir $@)

build/linux/arm64/e2e: $(GENERATED) $(SOURCES)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go test -c -o build/linux/arm64/$(notdir $@) $(BUILD_FLAGS) ./cmd/$(notdir $@)

build/linux/%: $(GENERATED) $(SOURCES)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build $(BUILD_FLAGS) -o build/linux/$(notdir $@) -ldflags "$(LDFLAGS)" ./cmd/$(notdir $@)

build/linux/amd64/%: go.mod $(SOURCES)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build $(BUILD_FLAGS) -o build/linux/amd64/$(notdir $@) -ldflags "$(LDFLAGS)" ./cmd/$(notdir $@)

build/linux/arm64/%: go.mod $(SOURCES)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build $(BUILD_FLAGS) -o build/linux/arm64/$(notdir $@) -ldflags "$(LDFLAGS)" ./cmd/$(notdir $@)

build/e2e: $(GENERATED) $(SOURCES)
	CGO_ENABLED=0 go test -c -o build/$(notdir $@) ./cmd/$(notdir $@)

build/%: $(GENERATED) $(SOURCES)
	CGO_ENABLED=0 go build -o build/$(notdir $@) $(BUILD_FLAGS) -ldflags "$(LDFLAGS)" ./cmd/$(notdir $@)

build.docker: build.linux build/linux/e2e
	docker build --rm -t "$(E2E_IMAGE):$(TAG)" -f Dockerfile.e2e --build-arg TARGETARCH= .
	docker build --rm -t "$(IMAGE):$(TAG)" -f Dockerfile --build-arg TARGETARCH= .

build.push: build.docker
	docker push "$(E2E_IMAGE):$(TAG)"
	docker push "$(IMAGE):$(TAG)"
