.PHONY: install run check

install:
	npm ci
	npm run build
	npm link

run:
	npm start

check:
	npm run check
	npm test

