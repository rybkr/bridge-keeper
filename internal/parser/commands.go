package parser

import (
	"bridgekeeper/internal/agents"
	"bridgekeeper/internal/env"
	"bridgekeeper/internal/policy"
	"fmt"
	"os"
	"strings"
)

func CommandENDPOINT() string {
	if strings.ToLower(env.Endpoint) == "ollama" {
		env.Endpoint = "Ollama"
		return PrettyPrintify("ENDPOINT", "Endpoint is now Ollama.")
	} else if strings.ToLower(env.Endpoint) == "gemini" {
		env.Endpoint = "Gemini"
		return PrettyPrintify("ENDPOINT", "Endpoint is now Google Gemini.")
	} else {
		return PrettyPrintify("ENDPOINT", "Uknown endpoint.\nOptions are:\n  - Gemini\n  - Ollama\n")
	}
}

func CommandLIST() string {
	if strings.ToLower(env.Endpoint) == "ollama" {
		return PrettyPrintify("MODELS", agents.OllamaLS())
	} else if strings.ToLower(env.Endpoint) == "gemini" {
		models, err := agents.RuntimeGeminiAgent.ListModels(env.ModelContext)
		if err != nil {
			return PrettyPrintify("ERROR", "Error fetching Gemini models")
		}
		return PrettyPrintify("MODELS", strings.Join(models, "\n"))
	} else {
		return PrettyPrintify("ENDPOINT", "Uknown endpoint.\nOptions are:\n  - Gemini\n  - Ollama\n")
	}
}

func CommandVERBOSE() string {
	env.Concise = env.Verbose
	env.Verbose = !env.Verbose
	CommandCLEAR()
	if env.Verbose {
		return PrettyPrintify("OPTIONS", "Verbose mode is now activated.")
	} else {
		return PrettyPrintify("OPTIONS", "Verbose mode is now deactivated.")
	}
}

func CommandCONCISE() string {
	env.Verbose = env.Concise
	env.Concise = !env.Concise
	CommandCLEAR()
	if env.Concise {
		return PrettyPrintify("OPTIONS", "Concise mode is now activated.")
	} else {
		return PrettyPrintify("OPTIONS", "Concise mode is now deactivated.")
	}
}

func CommandCLEAR() string {
	if strings.ToLower(env.Endpoint) == "ollama" {
		agents.OllamaConversation = []agents.Message{
			{
				Role:    "user",
				Content: env.SystemPrompt(),
			},
		}
	} else {
		agents.RuntimeGeminiAgent = agents.NewGeminiAgent()
	}
	return PrettyPrintify("OPTIONS", "Conversation history cleared.")
}

func CommandEXIT() {
	fmt.Println(PrettyPrintify(":)", "Goodbye!"))
	os.Exit(0)
}
func CommandSENDCHAT(input string) string {

	var resultstr string
	var err error
	if strings.ToLower(env.Endpoint) == "ollama" {
		resultstr, err = agents.SendOllamaMessage(input)
	} else {
		resultstr, err = agents.RuntimeGeminiAgent.SendGeminiMessage(input)
	}
	if err != nil {
		return PrettyPrintify("ERROR", resultstr)
	}
	return PrettyPrintify("MODEL", resultstr)
}

func CommandPOLICY() string {
	return PrettyPrintify("POLICY", policy.FormatPolicy(env.EPolicyFile))
}

func CommandTHINK() string {
	env.Thinking = !env.Thinking
	if env.Thinking {
		return PrettyPrintify("OPTIONS", "Thinking is now activated if the model supports it.")
	} else {
		return PrettyPrintify("OPTIONS", "Thinking is now deactivated.")
	}
}

func CommandHELP() string {
	return PrettyPrintify("HELP", MenuString)
}
