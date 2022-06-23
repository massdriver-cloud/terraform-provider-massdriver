TEST?=$$(go list ./... | grep -v 'vendor')
HOSTNAME=registry.terraform.io
NAMESPACE=massdriver-cloud
NAME=massdriver
BINARY=terraform-provider-${NAME}
VERSION=1.0.0
OS_ARCHS= darwin_amd64 linux_amd64

default: install

build:
	go build -o ${BINARY}

release:
	GOOS=darwin GOARCH=amd64 go build -o ./bin/${BINARY}_${VERSION}_darwin_amd64
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

.PHONY: infra.reset
infra.reset: infra.down infra.up ## Reset localstack infra

.PHONY: infra.down
infra.down: ## Destroy localstack infra
	-cd localstack && terraform destroy -auto-approve
	-cd localstack && rm terraform.tfstate*

.PHONY: infra.up
infra.up: ## Setup localstack for development
	cd localstack && terraform apply -auto-approve

testacc:
	TF_ACC=1 \
		MASSDRIVER_AWS_ENDPOINT=http://localhost:4566 \
		MASSDRIVER_EVENT_TOPIC_ARN=${shell make localstack.sns.last.arn} \
		go test $(TEST) -v $(TESTARGS) -timeout 120m

local.setup: install
	./test-setup.sh

local.apply: install
	cd dev; rm -rf .terraform .terraform.lock.hcl terraform.tfstate
	cd dev; terraform init
	cd dev; terraform apply

local.destroy:
	cd dev; terraform init
	cd dev; terraform destroy

local.sqs.poll:
	./sqspoll.sh

.PHONY: localstack.sns.list
localstack.sns.list: ## List sns topics from localstack
	@AWS_ACCESS_KEY_ID=test AWS_SECRET_ACCESS_KEY=test AWS_DEFAULT_REGION=us-east-1 \
  aws sns list-topics \
  --endpoint-url=http://localhost:4566

localstack.sns.last.arn: ## Get last topic arn created
	@make localstack.sns.list | jq '.Topics | last | .TopicArn'
