//go:build cxopcount

package main

import (
	"fmt"
	"os"

	"github.com/skycoin/skycoin-cx/t3-ast"
	vm "github.com/skycoin/skycoin-cx/t3-cx-runtime-vm"
)

// operationCountPrint writes the operations each loop ran to stderr.
func operationCountPrint() {
	fmt.Fprintf(os.Stderr, "cxopcount old %d vm %d\n", ast.OperationCount, vm.OperationCount)
}
