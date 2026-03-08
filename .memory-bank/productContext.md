# Product Context

## Why Muster Exists
Dynamic API interaction tool that generates CLI commands from OpenAPI specs instantly.

## Problem Solved
Developers need to call APIs from CLI without writing boilerplate code.
- Parse any OpenAPI spec
- Generate typed CLI commands
- Execute with proper auth/rate limiting
- Cache responses efficiently

## User Experience Goals
1. Single command: `muster generate <spec.yaml>`
2. Zero-config (smart defaults)
3. Interactive but scriptable
4. Shell completion built-in

## Current Stage
MVP Go CLI tool - not enterprise SaaS SDK platform.
