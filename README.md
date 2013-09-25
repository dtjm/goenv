goenv
=====
An environment for Go programming.

Run this:

    git clone git@github.com:dtjm/goenv
    cd goenv
    source goenv.sh
    
Put your source code into the `src` folder. If you have a package called
`mypackage`, put your code in `src/mypackage`. Any other packages
in this goenv can then `import mypackage`.

**goenv.sh** sets your GOPATH and installs `gocode`, `gocov` (code
 coverage), and `gocov-html` (code coverage output)

**Makefile** includes these targets:

- *clean*: Remove binaries and compiled packages
- *cover*: Run code coverage on all packages and renders it to `coverage/index.html`
- *test*: Run tests on all packages
