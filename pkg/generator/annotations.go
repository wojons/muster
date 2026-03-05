package generator

import (
	"fmt"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/openapi-cli/openapi-cli/pkg/openapi"
	"github.com/spf13/cobra"
)

// CommandAnnotationKey stores operation metadata in command annotations
const CommandAnnotationKey = "openapi.operation"

// GenerateCommandsWithAnnotations generates commands and attaches operation metadata
func (g *Generator) GenerateCommandsWithAnnotations(spec *openapi3.T) ([]*cobra.Command, []*openapi.Operation, error) {
	parser := &openapi.Parser{}
	ops, err := parser.ExtractOperations(spec)
	if err != nil {
		return nil, nil, fmt.Errorf("extract operations: %w", err)
	}

	groups := g.groupOperations(ops)
	var commands []*cobra.Command

	for resource, resourceOps := range groups {
		cmd := g.buildResourceCommandWithAnnotations(resource, resourceOps)
		commands = append(commands, cmd)
	}

	return commands, ops, nil
}

// buildResourceCommandWithAnnotations builds commands with operation annotations
func (g *Generator) buildResourceCommandWithAnnotations(resource string, operations []*openapi.Operation) *cobra.Command {
	cmd := &cobra.Command{
		Use:   resource,
		Short: fmt.Sprintf("Manage %s", resource),
		Long:  fmt.Sprintf("Commands for managing %s resources", resource),
	}

	for _, op := range operations {
		subCmd := g.buildOperationCommandWithAnnotation(op)
		cmd.AddCommand(subCmd)
	}

	return cmd
}

// buildOperationCommandWithAnnotation builds command with operation annotation
func (g *Generator) buildOperationCommandWithAnnotation(op *openapi.Operation) *cobra.Command {
	name := g.getCommandName(op)
	usage := g.buildUsage(name, op)
	longDesc := g.buildLongDescription(op)
	example := g.buildExample(name, op)

	cmd := &cobra.Command{
		Use:     usage,
		Short:   op.Summary,
		Long:    longDesc,
		Example: example,
		RunE:    g.createRunHandler(op),
	}

	// Store operation metadata in command annotation
	cmd.Annotations = make(map[string]string)
	cmd.Annotations[CommandAnnotationKey] = op.ID

	g.addFlags(cmd, op)

	return cmd
}
