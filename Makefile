.PHONY: build desk test check vet i18n test-go test-desk dev stop kill migrate help docker-up docker-down docker-logs docker-status docker-psql db-up db-down db-logs db-status db-psql

PORT ?= 8090

help: ## display this list of help commands
	@grep -E '^[a-zA-Z0-9_-]+:.*##' $(MAKEFILE_LIST) | sed 's/:.*##/ —/'

docker-up: ## start postgres in background via docker compose
	docker compose up -d

docker-down: ## stop and remove docker containers
	docker compose down

docker-logs: ## stream postgres logs in real time
	docker compose logs -f

docker-status: ## display docker containers status
	docker compose ps

docker-psql: ## open interactive psql terminal in development database (ddcore_dev)
	docker compose exec postgres psql -U ddcore -d ddcore_dev

db-up: docker-up ## alias for docker-up
db-down: docker-down ## alias for docker-down
db-logs: docker-logs ## alias for docker-logs
db-status: docker-status ## alias for docker-status
db-psql: docker-psql ## alias for docker-psql

build: desk ## compile desk + binary
	go build -o bin/ddcore ./cmd/ddcore

desk: ## compile desk (SvelteKit) into desk/build (embedded into binary)
	cd desk && npm install --silent && npm run build

check: ## check desk types and translation catalogs, without database
	cd desk && npm install --silent && npm run check
	./bin/ddcore types
	cd desk && npx tsc -p ../apps/demo/tsconfig.json --noEmit
	./bin/ddcore i18n extract --all --lang pt-BR --check

i18n: ## rewrite translations/<lang>.csv from code
	./bin/ddcore i18n extract --all --lang pt-BR

vet: ## Go static analysis
	go vet ./...

test-desk: ## svelte-check + desk unit tests
	cd desk && npm run check && npm run test

test: build vet ## run Go and desk tests
	go test ./internal/...
	./bin/ddcore test --app demo
	$(MAKE) test-desk

dev: ## start development server
	./bin/ddcore dev

stop: ## stop process listening on specified port (default PORT=8090, e.g.: make stop PORT=8090)
	@PID=$$(lsof -ti tcp:$(PORT) -sTCP:LISTEN 2>/dev/null); \
	if [ -n "$$PID" ]; then \
		echo "Stopping process(es) listening on port $(PORT) (PID: $$PID)..."; \
		kill -9 $$PID 2>/dev/null || true; \
		echo "Port $(PORT) freed."; \
	else \
		echo "No process listening on port $(PORT)."; \
	fi

kill: stop ## alias for stop

migrate: ## apply schema migrations
	./bin/ddcore migrate
