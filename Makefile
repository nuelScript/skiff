.PHONY: test vet build dash install

# Run the Go test suite.
test:
	go test ./...

vet:
	go vet ./...

# Build the CLI (uses the already-embedded dashboard).
build:
	go build -o skiff .

install:
	go install .

# Rebuild the dashboard and sync it into the Go binary's embed dir.
dash:
	cd web/dash && npm run build
	rm -rf internal/panel/dist && cp -r web/dash/dist internal/panel/dist
