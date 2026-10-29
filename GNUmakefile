default: lint test

build:
	go build -v ./...

lint:
	golangci-lint run

generate:
	go generate ./...

test:
	go test -race -cover ./...

testacc:
	TF_ACC=1 go test -v -cover -timeout 30m ./...

# The bundle the acceptance tests deploy, built with the nemo on the PATH.
testdata:
	cd testdata/bundles/hello && nemo bundle build -f compose.yaml --version 0.1.0 --image busybox:1.37=docker://busybox:1.37 -o bundle.json

.PHONY: build lint generate test testacc testdata
