package parser

import (
	"fmt"
	"log"
	"os"
	"strings"

	"golang.org/x/term"
)

// MenuString is the full menu as a string
var MenuString string = generateMenu(options)
var UserPrompt string = "››› "
var LongestMenuLineWidth int = 0
var MenuLinesHeight int = 0

// Minimum useful width of a terminal
const MinimumTerminalCharWidth = 25

// width or height of the terminal
var termWidth, termHeight, _ = term.GetSize(int(os.Stdout.Fd()))

var options []menuOption = []menuOption{
	{"help", "?", "Show help for a specific command"},
	{"list", "ls", "List all models"},
	{"verbose", "v", "Use verbose model output"},
	{"concise", "c", "Use concise model output"},
	{"conversational", "V", "Toggle if the model reads the whole conversation or last user message only"},
	{"clear", "R", "Clear conversation history"},
	{"policy", "po", "Show the policy"},
	{"exit", "bye", "Close the application"},
}

// MenuOption is an entry in the menu associated to an action
type menuOption struct {
	Name      string
	ShortName string
	Help      string
}

// String conversion for MenuOption
func (mo menuOption) String() string {
	// Example:
	// /help (/?) : Show help
	return fmt.Sprintf(
		"  %s (%s) : %s",
		mo.Name,
		mo.ShortName,
		mo.Help,
	)
}

// generates the menu as a string
func generateMenu(options []menuOption) string {
	output := strings.Builder{}
	output.WriteString("USAGE: command (alias) : Description\n")
	MenuLinesHeight += 1
	for _, opt := range options {
		line := opt.String() + "\n"
		if len(line) > LongestMenuLineWidth {
			LongestMenuLineWidth = len(line)
		}
		output.WriteString(line)
		MenuLinesHeight += 1
	}
	lastString := fmt.Sprintf("%s : %s", "<any other text input>", "Chat with the agent")
	output.WriteString(lastString)
	return output.String()
}

// Prints a pretty output
// `tag` is printed once in the top of the left margin
// `content` will be evaluated to match terminal width
func PrettyPrintify(tag string, content string) string {

	// The tag box
	topbld := strings.Builder{}
	btmbld := strings.Builder{}
	bld := strings.Builder{}
	midline := "║ "
	topbld.WriteString("╔═")
	btmbld.WriteString("╚═")
	for range len(tag) {
		topbld.WriteRune('═')
		btmbld.WriteRune('═')
		midline += " "
	}
	topbld.WriteString("═╦═")
	btmbld.WriteString("═╩═")
	midline += " ║ "

	// Parse the content strings into better lines
	tagged := false
	lcount := 0
	for val := range strings.SplitSeq(content, "\n") {
		newlines := splitLine(val, len(midline))
		for _, line := range newlines {
			thisline := strings.Builder{}
			if (lcount % 5) == 0 {
				tagged = false
				lcount = 0
			}
			lcount++
			if tagged {
				thisline.WriteString(midline)
			} else {
				thisline.WriteString("║ " + tag + " ║ ")
				tagged = true
			}
			thisline.WriteString(line)
			bld.WriteString(thisline.String())
			for range termWidth - thisline.Len() - 1 {
				bld.WriteRune(' ')
			}
			bld.WriteString(" ║\n")
		}

	}

	// Prepare the wraps
	for range maxLineLen(len(midline)) {
		topbld.WriteString("═")
		btmbld.WriteString("═")
	}
	topbld.WriteString("╗")
	btmbld.WriteString("╝\n")

	// Concatenate all!
	topbld.WriteString("\n")
	topbld.WriteString(bld.String())
	topbld.WriteString(btmbld.String())
	return topbld.String()
}

// given the width of the tag midline,
// determines how much space in the terminal remains
func maxLineLen(taglen int) int {
	newTermWidth, _, err := term.GetSize(int(os.Stdout.Fd()))
	termWidth = newTermWidth
	if err != nil {
		log.Fatal("Error getting terminal size")
	} else if termWidth < MinimumTerminalCharWidth {
		log.Fatal("Terminal not wide enough")
	}
	return termWidth - taglen
}

// Is whitespace?
func wsp(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

// Adds newlines to a line, assuming it has no newlines already
func splitLine(line string, taglen int) []string {
	bld := []string{}
	lineLen := maxLineLen(taglen) - 5
	counter := 0
	lastTokenStart := 0
	lastWsp := 0

	for ix, rn := range line {
		if counter == lineLen {
			bld = append(bld, line[lastTokenStart:lastWsp])
			lastTokenStart = lastWsp + 1
			counter = 0
		} else {
			counter++
		}
		if wsp(rn) {
			lastWsp = ix
		}
	}
	bld = append(bld, line[lastTokenStart:])
	return bld
}
