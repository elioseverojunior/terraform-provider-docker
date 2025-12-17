HOSTNAME=registry.terraform.io
NAMESPACE=elioseverojunior
NAME=docker
BINARY=terraform-provider-${NAME}
VERSION=0.1.0
OS_ARCH=$(shell go env GOOS)_$(shell go env GOARCH)

.PHONY: build install test testacc fmt vet clean tidy docs

build:
	go build -o ${BINARY}

install: build
	mkdir -p ~/.terraform.d/plugins/${HOSTNAME}/${NAMESPACE}/${NAME}/${VERSION}/${OS_ARCH}
	cp ${BINARY} ~/.terraform.d/plugins/${HOSTNAME}/${NAMESPACE}/${NAME}/${VERSION}/${OS_ARCH}/

test:
	go test ./... -v -timeout 120s

testacc:
	TF_ACC=1 go test ./... -v -timeout 120m

fmt:
	go fmt ./...

vet:
	go vet ./...

clean:
	rm -f ${BINARY}

tidy:
	go mod tidy

docs:
	tfplugindocs generate --provider-name ${NAME}

lint:
	golangci-lint run ./...

all: fmt vet build test docs
