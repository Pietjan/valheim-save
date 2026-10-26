# ==================================================================================== #
# CONFIG
# ==================================================================================== #

main_package_path = ./cmd/valheim-save
binary_name = valheim-save

# Where `make items` finds the running dev server and the game's text table:
# valheim-npc checked out beside this repo. Its tools/probe.sh compiles a probe
# and its server's query host runs it; its `make ui/meshes` leaves the English
# table under meshes/localization.
npc_dir = ../valheim-npc
query_port = 2460
english_json = ${npc_dir}/meshes/localization/English.json

# ==================================================================================== #
# HELPERS
# ==================================================================================== #

## help: print this help message
.PHONY: help
help:
	@echo 'Usage:'
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'

.PHONY: confirm
confirm:
	@echo -n 'Are you sure? [y/N] ' && read ans && [ $${ans:-N} = y ]

.PHONY: no-dirty
no-dirty:
	@test -z "$(shell git status --porcelain)"

# ==================================================================================== #
# QUALITY CONTROL
# ==================================================================================== #

## tidy: tidy modfiles and format .go files
.PHONY: tidy
tidy:
	go mod tidy -v
	go fmt ./...

## audit: run quality control checks
.PHONY: audit
audit:
	go mod tidy -diff
	go mod verify
	test -z "$(shell gofmt -l .)"
	go vet ./...
	go run honnef.co/go/tools/cmd/staticcheck@latest ./...
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...
	go test -race -buildvcs -vet=off ./...

## test: run all tests
.PHONY: test
test:
	go test -v -race -buildvcs ./...

## test/cover: run all tests and display coverage
.PHONY: test/cover
test/cover:
	go test -v -race -buildvcs -coverprofile=/tmp/coverage.out ./...
	go tool cover -html=/tmp/coverage.out

# ==================================================================================== #
# DEVELOPMENT
# ==================================================================================== #

## build: build the editor into bin/
.PHONY: build
build:
	go build -o=./bin/${binary_name} ${main_package_path}

## build/windows: build the editor for Windows into bin/
.PHONY: build/windows
build/windows:
	GOOS=windows GOARCH=amd64 go build -o=./bin/${binary_name}.exe ${main_package_path}

## run: build and run the editor, e.g. make run args="inspect hrafn"
.PHONY: run
run: build
	./bin/${binary_name} ${args}

## items: regenerate internal/character/items.tsv from the dev server in valheim-npc
# tools/item-dump.cs runs inside the server and writes its item prefabs to the
# server's /config, which is ${npc_dir}/docker/config on the host; the generator
# joins that with the game's English text. Data, so the output is committed;
# run after a game update.
.PHONY: items
items:
	"${npc_dir}/tools/probe.sh" tools/item-dump.cs "$${TMPDIR:-/tmp}/valheim-item-dump.dll"
	curl -sS --data-binary "@$${TMPDIR:-/tmp}/valheim-item-dump.dll" \
		"http://127.0.0.1:${query_port}/probe"
	@echo
	go run ./internal/character/items_gen.go \
		-items "${npc_dir}/docker/config/items.tsv" -english "${english_json}"
