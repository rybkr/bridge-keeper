package main

import (
	"fmt"
	"log"

	"bridgekeeper/internal/runtime"
)

/////// Toolchain placeholders ///////

func handleGoVersion(args map[string]any) (string, error) {
    // TODO: Replace with better result structs
    // or exec or something
    return "go version go1.25.7 linux/amd64", nil
}

func handleRustVersion(args map[string]any) (string, error) {
    // TODO: Replace with better result structs
    // or exec or something
    return "cargo 1.65.0", nil
}

var toolchain = []runtime.ToolDef{
    {
        Name: "go_version",
        Description: "Get the current version of go",
        Handler: handleGoVersion,
    },
        {
        Name: "rust_version",
        Description: "Get the current version of Rust",
        Handler: handleRustVersion,
    },
}

/// Deferred shutdown ///
func deferredShutdown() {
    if err := runtime.Shutdown(); nil != err {
        log.Printf("shutdown %v", err)
    }
}

/////// MAIN ///////
func main() {

    if err := runtime.Initialize(11434); nil != err {
        log.Fatalf("Could not initialize: %w", err)
    }

    // Call the anonymous function once main exits scope
    defer deferredShutdown()

    // simple tests - replace with filtered promtps later
    prompts := []string {
        "Get the version of Go",
        "Get the version of rust with Cargo",
    }

    // send the list of prompts
    for _, prompt := range prompts {
        fmt.Printf("\n> %s\n", prompt)
        response, err := runtime.QueryWithTools(prompt, toolchain)
        if nil != err {
            log.Printf("Query error %v", err)
            continue // attempt other prompts
        }
        fmt.Printf("< %s\n", response)
    }
}
