# Muster Deployment Playbooks

This directory contains deployment playbooks for each environment.

## Playbooks

- `deploy-dev.md` - Development environment deployment
- `deploy-staging.md` - Staging environment deployment
- `deploy-prod.md` - Production environment deployment

## Quick Reference

```bash
# Development
cat playbooks/deploy-dev.md

# Staging
cat playbooks/deploy-staging.md

# Production
cat playbooks/deploy-prod.md
```

## Pre-deployment Checklist

Before any deployment:

1. [ ] Review release notes
2. [ ] Check CI/CD pipeline status
3. [ ] Verify database migrations (if any)
4. [ ] Confirm rollback plan
5. [ ] Notify team (for staging/prod)

## Post-deployment Verification

After any deployment:

1. [ ] Health check passes
2. [ ] Smoke tests pass
3. [ ] Metrics flowing
4. [ ] Logs show no errors
5. [ ] User acceptance tests pass (staging/prod)
