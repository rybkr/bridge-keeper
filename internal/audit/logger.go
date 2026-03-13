package audit

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
)

////////////////////  Types and globals  ////////////////////

// Severity as a typed int and implement the Stringer interface
type Severity int

const (
	Debug Severity = iota // Most verbose
	Info                  // Default
	Warning
	Error
)

func (s Severity) String() string {
	return [...]string{"DEBUG", "INFO", "WARN", "ERROR"}[s]
}

// System shared severity level
var systemLogLevel Severity = Info

// Log file should be open for whole runtime
var logFile *os.File

// //////////////////  Initialization  ////////////////////
// Runs at the beginning of module lifetime in program
func init() {
	err := os.MkdirAll("./logs", 0755)
	if err != nil {
		return
	}
	// path to log file                                     // YYYY-MM-DD
	path := fmt.Sprintf("./logs/%s-log.md", time.Now().Format("2006-01-02-15-04-05"))

	// Check if the file exists before trying to open
	_, stat := os.Stat(path)
	isNewFile := os.IsNotExist(stat)

	// Open the file, creating if it doesn't exist, in write-only append mode
	logFile, err = os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}

	// Add a header if new
	if isNewFile {
		logFile.WriteString(fmt.Sprintf("# Log — %s\n\n| Time | Module | Function | Severity | Message |\n|------|--------|----------|----------|---------|\n", time.Now().Format("2006-01-02-15-04-05")))
	}
}

// //////////////////  Super awesome all in one log function  ////////////////////
func LogEvent(message string, severity ...Severity) {

	// Capture the time ASAP
	eventTime := time.Now().Format("2006-01-02 15:04:05")

	// determine severity to log at, and if it's too low, skip
	thisSeverity := Info
	if len(severity) > 0 {
		thisSeverity = severity[0]
	}
	if thisSeverity < systemLogLevel || logFile == nil {
		return
	}

	// runtime.Caller(1) will get the caller module
	pc, rcFile, _, rcOk := runtime.Caller(1)
	module, function := "unknown module", "unknown function"
	if rcOk {
		// Use the file name as the module name
		base := rcFile[strings.LastIndex(rcFile, "/")+1:]
		module = strings.TrimSuffix(base, ".go")

		// FuncForPC returns "pkg.<name of function>"
		if rcFunc := runtime.FuncForPC(pc); rcFunc != nil {
			fullFuncName := rcFunc.Name()
			// taking the last part for just the function name
			function = fullFuncName[strings.LastIndex(fullFuncName, ".")+1:]
		}
	}

	// This creates a markdown table in the log
	logEntry := fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
		eventTime,
		module,
		function,
		thisSeverity,
		strings.ReplaceAll(message, "|", "¦"), // replace line chars with broken line bc of markdown output
	)

	logFile.WriteString(logEntry)
}
