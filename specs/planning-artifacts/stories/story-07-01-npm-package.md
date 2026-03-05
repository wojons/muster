# Story 7.1: npm Package Setup

## Epic
Epic 7: Distribution & Packaging

## Story
As a user, I want to install the CLI via npm so that I can easily manage versions and updates.

## Acceptance Criteria
- [ ] Create package.json with proper metadata
- [ ] Set up bin entry point
- [ ] Configure TypeScript compilation
- [ ] Set up build scripts
- [ ] Create .npmignore
- [ ] Test local npm install
- [ ] Prepare for npm publish

## Technical Notes
- Package name: `openapi-cli` or `@scope/openapi-cli`
- Entry point: `dist/cli.js`
- Include type definitions
- Set minimum Node.js version (18+)

## Estimation
- **Story Points**: 2
- **Duration**: 1-2 days

## Dependencies
- Core functionality implemented

## Test Cases
1. `npm install` works locally
2. `npx openapi-cli --version` works
3. Global install works: `npm install -g openapi-cli`
4. Binary is in PATH after global install
5. Package includes all necessary files
6. Dependencies correctly specified
