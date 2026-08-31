// PEPA FluxCD Plugin — implements CDEngineProvider for FluxCD GitOps.
package main

import (
	sdk "github.com/pepa/pepa/internal/plugin/sdk-go"
)

func main() {
	sdk.Serve(&FluxCDPlugin{})
}
