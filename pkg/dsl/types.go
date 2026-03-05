package dsl

import (
	"go.starlark.net/syntax"
)

type Script struct {
	Name        string
	Description string
	Version     string
	Metadata    *ScriptMetadata
	AST         *syntax.File
	Source      string
	Globals     map[string]interface{}
}

type ScriptMetadata struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Version     string            `json:"version"`
	Author      string            `json:"author"`
	Inputs      []Input           `json:"inputs,omitempty"`
	Outputs     []Output          `json:"outputs,omitempty"`
	Tags        map[string]string `json:"tags,omitempty"`
}

type Input struct {
	Name        string      `json:"name"`
	Type        string      `json:"type"`
	Required    bool        `json:"required"`
	Default     interface{} `json:"default,omitempty"`
	Description string      `json:"description"`
}

type Output struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

type ScriptResult struct {
	Output   interface{} `json:"output"`
	ExitCode int         `json:"exit_code"`
	Error    string      `json:"error,omitempty"`
	Duration string      `json:"duration"`
}

type ExecutionContext struct {
	Inputs    map[string]interface{}
	Config    *EngineConfig
	Secrets   map[string]string
	Variables map[string]interface{}
	Env       map[string]string
}

type EngineConfig struct {
	MaxSteps         int64
	MaxExecutionTime int
	MaxMemoryMB      int
	EnableNetwork    bool
	AllowedHosts     []string
}

type ErrorType string

const (
	ErrorParse      ErrorType = "parse"
	ErrorRuntime    ErrorType = "runtime"
	ErrorValidation ErrorType = "validation"
	ErrorResource   ErrorType = "resource"
)

type DSLError struct {
	Type    ErrorType `json:"type"`
	Message string    `json:"message"`
	Line    int       `json:"line,omitempty"`
	Column  int       `json:"column,omitempty"`
}

func (e *DSLError) Error() string {
	if e.Line > 0 {
		return e.Message
	}
	return e.Message
}

func NewDefaultEngineConfig() *EngineConfig {
	return &EngineConfig{
		MaxSteps:         100000,
		MaxExecutionTime: 30,
		MaxMemoryMB:      256,
		EnableNetwork:    true,
		AllowedHosts:     []string{},
	}
}
