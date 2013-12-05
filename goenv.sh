export GOPATH=`pwd`
echo "export GOPATH=$GOPATH"

export PATH=$GOPATH/bin:$PATH
echo "export PATH=$GOPATH/bin:\$PATH"

GOBIN=./bin

if [ ! -x bin/gocode ]; then
    echo "installing gocode into $GOBIN"
    GOBIN=$GOBIN go get github.com/nsf/gocode
fi
