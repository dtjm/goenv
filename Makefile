cover: coverage/index.html
coverage/index.json: bin/gocov src/sendlib/**/*.go
	mkdir -p coverage
	bin/gocov test ... > coverage/index.json
	bin/gocov report < coverage/index.json
coverage/index.html: bin/gocov-html coverage/index.json
	mkdir -p coverage
	bin/gocov-html < coverage/index.json > coverage/index.html
bin/gocov:
	. goenv.sh
bin/gocov-html:
	. goenv.sh

test:
	go test ...
clean:
	rm -rf coverage pkg bin
version:
	go version
