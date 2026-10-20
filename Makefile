.PHONY: check-backend check-frontend check hooks

check-backend:
	cd server && test -z "$$(gofmt -l cmd internal)"
	cd server && go vet ./...
	cd server && go test -race -count=1 ./...

check-frontend:
	cd client && npm run lint
	cd client && npm run format:check
	cd client && npm test
	cd client && npm run build

check: check-backend check-frontend

hooks:
	git config core.hooksPath .githooks

