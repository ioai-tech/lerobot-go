.PHONY: test lint build e2e examples official-stability

BINARY := lerobot-go

test:
	go test ./... -race -count=1

lint:
	golangci-lint run

build:
	go build -o $(BINARY) ./cmd/lerobot-go

e2e:
	./scripts/run_e2e_write_test.sh

examples:
	go run ./examples/write_v30
	go run ./examples/validate_dataset ./testdata/output/v30

official-stability:
	./scripts/run_official_stability_checks.sh
