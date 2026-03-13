package main

import (
	"bridgekeeper/internal/audit"
	"fmt"
)

func main() {
	fmt.Println("bridgekeeper")
	audit.LogEvent("MainMessage", audit.Warning)
}
