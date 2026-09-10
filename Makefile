.PHONY: build desk test check vet test-go test-desk dev stop kill migrate help docker-up docker-down docker-logs docker-status docker-psql db-up db-down db-logs db-status db-psql

PORT ?= 8090

help: ## exibe esta lista de comandos de ajuda
	@grep -E '^[a-zA-Z0-9_-]+:.*##' $(MAKEFILE_LIST) | sed 's/:.*##/ —/'

docker-up: ## sobe o postgres em background via docker compose
	docker compose up -d

docker-down: ## para e remove os containers docker
	docker compose down

docker-logs: ## visualiza os logs do postgres em tempo real
	docker compose logs -f

docker-status: ## exibe o status dos containers docker
	docker compose ps

docker-psql: ## abre terminal psql interativo no banco de desenvolvimento (ddcore_dev)
	docker compose exec postgres psql -U ddcore -d ddcore_dev

db-up: docker-up ## alias para docker-up
db-down: docker-down ## alias para docker-down
db-logs: docker-logs ## alias para docker-logs
db-status: docker-status ## alias para docker-status
db-psql: docker-psql ## alias para docker-psql

build: desk ## compila desk + binário
	go build -o bin/ddcore ./cmd/ddcore

desk: ## compila o desk (SvelteKit) para desk/build (embutido no binário)
	cd desk && npm install --silent && npm run build

check: ## verifica os tipos do desk, sem banco
	cd desk && npm install --silent && npm run check
	./bin/ddcore types
	cd desk && npx tsc -p ../apps/exemplo/tsconfig.json --noEmit

vet: ## análise estática do Go
	go vet ./...

test-desk: ## svelte-check + testes unitários do desk
	cd desk && npm run check && npm run test

test: build vet ## testes Go e do desk
	go test ./internal/...
	./bin/ddcore test --app exemplo
	$(MAKE) test-desk

dev: ## servidor de desenvolvimento
	./bin/ddcore dev

stop: ## encerra o processo rodando na porta especificada (padrão PORT=8090, ex: make stop PORT=8090)
	@PID=$$(lsof -ti tcp:$(PORT) -sTCP:LISTEN 2>/dev/null); \
	if [ -n "$$PID" ]; then \
		echo "Encerrando processo(s) escutando na porta $(PORT) (PID: $$PID)..."; \
		kill -9 $$PID 2>/dev/null || true; \
		echo "Porta $(PORT) liberada."; \
	else \
		echo "Nenhum processo escutando na porta $(PORT)."; \
	fi

kill: stop ## alias para stop

migrate: ## aplica as migrações de schema
	./bin/ddcore migrate
