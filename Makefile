TEST?=$$(go list ./... | grep -v 'vendor')
HOSTNAME=registry.terraform.io
NAMESPACE=massdriver-cloud
NAME=massdriver
BINARY=terraform-provider-${NAME}
VERSION=2.0.0
OS_ARCHS= darwin_amd64 darwin_arm64 linux_amd64

default: install

build:
	go build -o ${BINARY}

release:
	GOOS=darwin GOARCH=amd64 go build -o ./bin/${BINARY}_${VERSION}_darwin_amd64
	GOOS=darwin GOARCH=arm64 go build -o ./bin/${BINARY}_${VERSION}_darwin_arm64
	GOOS=freebsd GOARCH=386 go build -o ./bin/${BINARY}_${VERSION}_freebsd_386
	GOOS=freebsd GOARCH=amd64 go build -o ./bin/${BINARY}_${VERSION}_freebsd_amd64
	GOOS=freebsd GOARCH=arm go build -o ./bin/${BINARY}_${VERSION}_freebsd_arm
	GOOS=linux GOARCH=386 go build -o ./bin/${BINARY}_${VERSION}_linux_386
	GOOS=linux GOARCH=amd64 go build -o ./bin/${BINARY}_${VERSION}_linux_amd64
	GOOS=linux GOARCH=arm go build -o ./bin/${BINARY}_${VERSION}_linux_arm
	GOOS=openbsd GOARCH=386 go build -o ./bin/${BINARY}_${VERSION}_openbsd_386
	GOOS=openbsd GOARCH=amd64 go build -o ./bin/${BINARY}_${VERSION}_openbsd_amd64
	GOOS=solaris GOARCH=amd64 go build -o ./bin/${BINARY}_${VERSION}_solaris_amd64
	GOOS=windows GOARCH=386 go build -o ./bin/${BINARY}_${VERSION}_windows_386
	GOOS=windows GOARCH=amd64 go build -o ./bin/${BINARY}_${VERSION}_windows_amd64

install: build
	for arch in $(OS_ARCHS) ; do\
		echo $${arch} ; \
		mkdir -p ~/.terraform.d/plugins/${HOSTNAME}/${NAMESPACE}/${NAME}/${VERSION}/$${arch} ;\
		cp ${BINARY} ~/.terraform.d/plugins/${HOSTNAME}/${NAMESPACE}/${NAME}/${VERSION}/$${arch} ;\
	done

	rm ${BINARY}

test:
	go test -i $(TEST) || exit 1
	echo $(TEST) | xargs -t -n4 go test $(TESTARGS) -timeout=30s -parallel=4

.PHONY: docs
docs: ## Generate documentation
	@echo "Generating documentation..."
	@# Pinned to v0.20.1 — v0.21.0+ requires Go >= 1.25.
	@go install github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@v0.20.1
	@tfplugindocs generate --provider-name=massdriver --examples-dir=./examples

.PHONY: lint
lint: ## Run golangci-lint (same version as CI)
	@echo "Linting..."
	@# Pinned to match .github/workflows/lint.yml.
	@go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.1.6
	@golangci-lint run ./...