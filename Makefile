MODULES := . postgres

.PHONY: all lint fmt test db-up

all: lint test

lint:
	@for m in $(MODULES); do (cd $$m && golangci-lint run ./...) || exit 1; done

fmt:
	@for m in $(MODULES); do (cd $$m && golangci-lint fmt ./...) || exit 1; done

test: db-up
	@for m in $(MODULES); do (cd $$m && go test -race -count=1 ./...) || exit 1; done

db-up:
	@docker compose up -d --wait
