package flags

import (
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// URLValidator validates URL strings.
// It checks that the URL is well-formed and has a valid scheme.
type URLValidator struct {
	// AllowedSchemes restricts valid URL schemes. If empty, all schemes are allowed.
	AllowedSchemes []string

	// RequireHost ensures the URL has a host component.
	RequireHost bool

	// AllowRelative allows relative URLs (without scheme).
	AllowRelative bool
}

// NewURLValidator creates a new URLValidator with default settings.
func NewURLValidator() *URLValidator {
	return &URLValidator{
		AllowedSchemes: []string{},
		RequireHost:    true,
		AllowRelative:  false,
	}
}

// Validate checks if the value is a valid URL.
func (v *URLValidator) Validate(value string) error {
	if value == "" {
		return fmt.Errorf("URL cannot be empty")
	}

	u, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Check for relative URLs
	if u.Scheme == "" {
		if !v.AllowRelative {
			return fmt.Errorf("URL must have a scheme (e.g., http://, https://)")
		}
		// For relative URLs, we don't need further validation
		return nil
	}

	// Check allowed schemes
	if len(v.AllowedSchemes) > 0 {
		found := false
		for _, scheme := range v.AllowedSchemes {
			if strings.EqualFold(u.Scheme, scheme) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("URL scheme '%s' is not allowed, must be one of: %s",
				u.Scheme, strings.Join(v.AllowedSchemes, ", "))
		}
	}

	// Check host requirement
	if v.RequireHost && u.Host == "" {
		return fmt.Errorf("URL must have a host component")
	}

	return nil
}

// Name returns the validator's name.
func (v *URLValidator) Name() string {
	return "url"
}

// IPValidator validates IP addresses (IPv4 or IPv6).
type IPValidator struct {
	// AllowIPv4 allows IPv4 addresses.
	AllowIPv4 bool

	// AllowIPv6 allows IPv6 addresses.
	AllowIPv6 bool

	// AllowPrivate allows private/reserved IP ranges.
	AllowPrivate bool

	// AllowLoopback allows loopback addresses (127.0.0.1, ::1).
	AllowLoopback bool
}

// NewIPValidator creates a new IPValidator allowing both IPv4 and IPv6.
func NewIPValidator() *IPValidator {
	return &IPValidator{
		AllowIPv4:     true,
		AllowIPv6:     true,
		AllowPrivate:  true,
		AllowLoopback: true,
	}
}

// NewIPv4Validator creates an IPValidator that only allows IPv4 addresses.
func NewIPv4Validator() *IPValidator {
	return &IPValidator{
		AllowIPv4:     true,
		AllowIPv6:     false,
		AllowPrivate:  true,
		AllowLoopback: true,
	}
}

// NewIPv6Validator creates an IPValidator that only allows IPv6 addresses.
func NewIPv6Validator() *IPValidator {
	return &IPValidator{
		AllowIPv4:     false,
		AllowIPv6:     true,
		AllowPrivate:  true,
		AllowLoopback: true,
	}
}

// Validate checks if the value is a valid IP address.
func (v *IPValidator) Validate(value string) error {
	if value == "" {
		return fmt.Errorf("IP address cannot be empty")
	}

	ip := net.ParseIP(value)
	if ip == nil {
		return fmt.Errorf("invalid IP address: %s", value)
	}

	// Check IPv4 vs IPv6
	isIPv4 := ip.To4() != nil
	if isIPv4 && !v.AllowIPv4 {
		return fmt.Errorf("IPv4 addresses are not allowed: %s", value)
	}
	if !isIPv4 && !v.AllowIPv6 {
		return fmt.Errorf("IPv6 addresses are not allowed: %s", value)
	}

	// Check loopback
	if ip.IsLoopback() && !v.AllowLoopback {
		return fmt.Errorf("loopback addresses are not allowed: %s", value)
	}

	// Check private ranges
	if !v.AllowPrivate {
		if isPrivate(ip) {
			return fmt.Errorf("private IP addresses are not allowed: %s", value)
		}
	}

	return nil
}

// isPrivate checks if an IP is in a private range.
func isPrivate(ip net.IP) bool {
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"fc00::/7",
	}

	for _, cidr := range privateRanges {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if ipNet.Contains(ip) {
			return true
		}
	}

	return false
}

// Name returns the validator's name.
func (v *IPValidator) Name() string {
	return "ip"
}

// EmailValidator validates email addresses.
type EmailValidator struct {
	// AllowedDomains restricts valid email domains. If empty, all domains are allowed.
	AllowedDomains []string

	// RequireDomain requires the email to have a domain part.
	RequireDomain bool
}

// NewEmailValidator creates a new EmailValidator with default settings.
func NewEmailValidator() *EmailValidator {
	return &EmailValidator{
		AllowedDomains: []string{},
		RequireDomain:  true,
	}
}

// Validate checks if the value is a valid email address.
func (v *EmailValidator) Validate(value string) error {
	if value == "" {
		return fmt.Errorf("email address cannot be empty")
	}

	// Use mail.ParseAddress for RFC-compliant parsing
	addr, err := mail.ParseAddress(value)
	if err != nil {
		return fmt.Errorf("invalid email address: %w", err)
	}

	email := addr.Address

	// Check for domain requirement
	if v.RequireDomain {
		parts := strings.Split(email, "@")
		if len(parts) != 2 || parts[1] == "" {
			return fmt.Errorf("email address must have a valid domain")
		}

		// Check allowed domains
		if len(v.AllowedDomains) > 0 {
			domain := parts[1]
			found := false
			for _, allowed := range v.AllowedDomains {
				if strings.EqualFold(domain, allowed) {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("email domain '%s' is not allowed, must be one of: %s",
					domain, strings.Join(v.AllowedDomains, ", "))
			}
		}
	}

	return nil
}

// Name returns the validator's name.
func (v *EmailValidator) Name() string {
	return "email"
}

// PathValidator validates file system paths.
type PathValidator struct {
	// MustExist requires the path to exist.
	MustExist bool

	// MustBeFile requires the path to be a regular file.
	MustBeFile bool

	// MustBeDir requires the path to be a directory.
	MustBeDir bool

	// MustBeReadable requires the path to be readable.
	MustBeReadable bool

	// MustBeWritable requires the path to be writable.
	MustBeWritable bool

	// AllowRelative allows relative paths. If false, only absolute paths are allowed.
	AllowRelative bool

	// ResolveSymlinks resolves symlinks before validation.
	ResolveSymlinks bool
}

// NewPathValidator creates a new PathValidator with default settings.
func NewPathValidator() *PathValidator {
	return &PathValidator{
		MustExist:       false,
		MustBeFile:      false,
		MustBeDir:       false,
		MustBeReadable:  false,
		MustBeWritable:  false,
		AllowRelative:   true,
		ResolveSymlinks: false,
	}
}

// NewExistingFileValidator creates a PathValidator for existing readable files.
func NewExistingFileValidator() *PathValidator {
	return &PathValidator{
		MustExist:      true,
		MustBeFile:     true,
		MustBeReadable: true,
		AllowRelative:  true,
	}
}

// NewExistingDirValidator creates a PathValidator for existing readable directories.
func NewExistingDirValidator() *PathValidator {
	return &PathValidator{
		MustExist:      true,
		MustBeDir:      true,
		MustBeReadable: true,
		AllowRelative:  true,
	}
}

// NewWritablePathValidator creates a PathValidator for writable paths.
func NewWritablePathValidator() *PathValidator {
	return &PathValidator{
		MustBeWritable: true,
		AllowRelative:  true,
	}
}

// Validate checks if the value is a valid file system path.
func (v *PathValidator) Validate(value string) error {
	if value == "" {
		return fmt.Errorf("path cannot be empty")
	}

	path := value

	// Resolve symlinks if requested
	if v.ResolveSymlinks {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return fmt.Errorf("failed to resolve symlinks in path '%s': %w", path, err)
		}
		path = resolved
	}

	// Check for absolute path requirement
	if !v.AllowRelative && !filepath.IsAbs(path) {
		return fmt.Errorf("path must be absolute, got relative path: %s", path)
	}

	// Clean the path
	path = filepath.Clean(path)

	// Check existence if required
	if v.MustExist {
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("path does not exist: %s", path)
			}
			return fmt.Errorf("failed to access path '%s': %w", path, err)
		}

		// Check file vs directory
		if v.MustBeFile && info.IsDir() {
			return fmt.Errorf("path must be a file, but is a directory: %s", path)
		}
		if v.MustBeDir && !info.IsDir() {
			return fmt.Errorf("path must be a directory, but is a file: %s", path)
		}

		// Check readability
		if v.MustBeReadable {
			file, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("path is not readable: %s", path)
			}
			_ = file.Close()
		}
	}

	// Check writability
	if v.MustBeWritable {
		// Try to open for writing (create if doesn't exist, append if does)
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("path is not writable: %s", path)
		}
		_ = file.Close()

		// If we created the file and it shouldn't exist, remove it
		if !v.MustExist {
			_ = os.Remove(path)
		}
	}

	return nil
}

// Name returns the validator's name.
func (v *PathValidator) Name() string {
	return "path"
}

// Common Validators

// URL is a convenience function that returns a default URLValidator.
func URL() Validator {
	return NewURLValidator()
}

// URLWithSchemes creates a URLValidator restricted to specific schemes.
func URLWithSchemes(schemes ...string) Validator {
	return &URLValidator{
		AllowedSchemes: schemes,
		RequireHost:    true,
		AllowRelative:  false,
	}
}

// IP is a convenience function that returns a default IPValidator.
func IP() Validator {
	return NewIPValidator()
}

// IPv4 creates an IPValidator that only allows IPv4 addresses.
func IPv4() Validator {
	return NewIPv4Validator()
}

// IPv6 creates an IPValidator that only allows IPv6 addresses.
func IPv6() Validator {
	return NewIPv6Validator()
}

// Email is a convenience function that returns a default EmailValidator.
func Email() Validator {
	return NewEmailValidator()
}

// EmailWithDomains creates an EmailValidator restricted to specific domains.
func EmailWithDomains(domains ...string) Validator {
	return &EmailValidator{
		AllowedDomains: domains,
		RequireDomain:  true,
	}
}

// Path is a convenience function that returns a default PathValidator.
func Path() Validator {
	return NewPathValidator()
}

// ExistingFile creates a PathValidator for existing readable files.
func ExistingFile() Validator {
	return NewExistingFileValidator()
}

// ExistingDir creates a PathValidator for existing readable directories.
func ExistingDir() Validator {
	return NewExistingDirValidator()
}

// WritablePath creates a PathValidator for writable paths.
func WritablePath() Validator {
	return NewWritablePathValidator()
}
