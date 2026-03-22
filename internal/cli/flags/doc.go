// Package flags provides a type-safe flag parsing system with validation
// for the Muster CLI framework.
//
// The flags package extends the standard flag parsing capabilities with
// custom types, validators, and integration with the Cobra CLI framework.
// It supports various flag types including strings, integers, booleans,
// URLs, IP addresses, email addresses, and file paths.
//
// Basic usage:
//
//	parser := flags.NewParser()
//	parser.AddFlag(flags.FlagDefinition{
//	    Name:     "host",
//	    Type:     flags.FlagTypeString,
//	    Required: true,
//	    Validator: flags.NewIPValidator(),
//	})
//
// Advanced usage with custom validation:
//
//	parser.AddFlag(flags.FlagDefinition{
//	    Name:     "endpoint",
//	    Type:     flags.FlagTypeURL,
//	    Required: true,
//	    Validator: flags.NewURLValidator(
//	        flags.URLRequireHTTPS(),
//	        URLDisallowLocalhost(),
//	    ),
//	})
//
// Integration with Cobra:
//
//	cmd := &cobra.Command{
//	    RunE: func(cmd *cobra.Command, args []string) error {
//	        return parser.Bind(cmd.Flags())
//	    },
//	}
//
// The package provides built-in validators for common use cases:
//   - URLValidator: Validates URL format and scheme
//   - IPValidator: Validates IPv4 and IPv6 addresses
//   - EmailValidator: Validates email address format
//   - PathValidator: Validates file system paths
//
// All validators can be composed using the Compose() method for complex
// validation scenarios.
package flags
