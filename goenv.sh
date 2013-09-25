export GOPATH=`pwd`
echo "export GOPATH=$GOPATH"

export PATH=$GOPATH/bin:$PATH
echo "export PATH=$GOPATH/bin:\$PATH"

GOBIN=./bin

if [ ! -x bin/gocode ]; then
    echo "installing gocode into $GOBIN"
    GOBIN=$GOBIN go get github.com/nsf/gocode
fi

if [ ! -x bin/gocov ]; then
    echo "installing gocov into $GOBIN"
    GOBIN=$GOBIN go get github.com/axw/gocov/gocov
fi

if [ ! -x bin/gocov-html ]; then
    echo "installing gocov-html into $GOBIN"
    GOBIN=$GOBIN go get github.com/matm/gocov-html
fi
