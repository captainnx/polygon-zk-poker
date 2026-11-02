NARGO ?= $(CURDIR)/.tools/nargo
FORGE ?= $(CURDIR)/node_modules/.bin/forge

.PHONY: test test-go test-noir test-contracts test-ts fuzz demo verify primitive-proof primitive-evm up down bootstrap compile-hand test-hand prepare-hand signed-demo differential full-e2e deploy-local deploy-amoy deploy-polygon
test: test-go test-ts test-noir test-contracts test-hand
test-go:
	go test -race ./apps/server/...
	go vet ./apps/server/...
test-ts:
	npm run build
	npm test
test-noir:
	$(NARGO) test --workspace --program-dir circuits
	$(NARGO) test --program-dir apps/prover/circuit
test-contracts:
	cd contracts && $(FORGE) test -vv
fuzz:
	cd apps/server && go test ./internal/engine -run '^$$' -fuzz '^FuzzHandConservation$$' -fuzztime 30s -parallel 4
	cd apps/server && go test ./internal/engine -run '^$$' -fuzz '^FuzzRejectedActionsAtomic$$' -fuzztime 15s -parallel 4
demo:
	go run ./apps/server/cmd/simulate > reports/hand.json
	go run ./apps/server/cmd/verify reports/hand.json
verify:
	go run ./apps/server/cmd/verify fixtures/nine-player-hand.json
	go run ./apps/server/cmd/verify-signed fixtures/signed-hand-v3.json
bootstrap:
	npm ci
	bash scripts/install-noir.sh
	bash scripts/install-bb.sh
compile-hand:
	$(NARGO) compile --program-dir apps/prover/hand
test-hand: compile-hand
	node apps/prover/src/cli.ts test-hand fixtures/signed-hand-v3.json
	node scripts/signed-adversarial.ts --fixture
	go run ./apps/server/cmd/verify-signed reports/streets-signed-hand.json
prepare-hand: compile-hand
	node apps/prover/src/cli.ts prepare-hand
signed-demo:
	node scripts/signed-nine.ts
	go run ./apps/server/cmd/verify-signed reports/signed-hand.json
differential:
	go run ./apps/server/cmd/engine-vectors > fixtures/engine-vectors.json
	$(NARGO) compile --program-dir apps/prover/engine-diff
	node scripts/engine-diff.ts
full-e2e: prepare-hand
	docker compose up -d --build
	node scripts/full-e2e.ts
deploy-local:
	node --env-file-if-exists=.env scripts/deploy.ts local
deploy-amoy:
	node --env-file-if-exists=.env scripts/deploy.ts amoy
deploy-polygon:
	node --env-file-if-exists=.env scripts/deploy.ts polygon
primitive-proof:
	$(NARGO) compile --program-dir apps/prover/circuit
	node apps/prover/src/primitive.ts
primitive-evm: primitive-proof
	node scripts/primitive-evm.ts
up:
	docker compose up -d --build
down:
	docker compose down
