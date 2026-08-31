package main

import (
	sdk "github.com/pepa/pepa/internal/plugin/sdk-go"
)

func main() {
	sdk.Serve(&ProxmoxPlugin{})
}
