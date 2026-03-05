# Epic 5: Configuration Management

## Overview
Implement a flexible configuration system supporting profiles, environment variables, secrets management, and per-API overrides.

## Business Value
- Essential for production usage with multiple environments
- Security through proper secret handling
- Reduces repetitive command-line arguments
- Enables team collaboration through shared configs

## Acceptance Criteria
1. Load configuration from files (.openapi-cli.yaml)
2. Environment variable substitution
3. Profile management (dev, staging, prod)
4. Secret integration (1Password, Vault)
5. Per-API configuration overrides
6. Configuration validation
7. Interactive configuration wizard

## Technical Requirements
- YAML/JSON config parsing
- Environment variable interpolation
- Hierarchical config merging
- Secret provider plugins
- Config schema validation
- Secure secret storage

## Dependencies
- Epic 2: CLI Generation (config affects CLI behavior)
- Epic 3: Request Execution (config provides auth/base URLs)

## Stories
- Story 5.1: Config file loader
- Story 5.2: Environment variable handling
- Story 5.3: Profile management
- Story 5.4: Secret provider interface
- Story 5.5: 1Password integration
- Story 5.6: Config validation
- Story 5.7: Interactive config wizard

## Estimation
- **Story Points**: 8
- **Duration**: 1-2 weeks

## Risks
- Config file format debates
- Secret provider API changes
- Environment variable name collisions

## Notes
- Follow XDG Base Directory Specification
- Support .env files for simple cases
- Consider encrypted config files for teams
