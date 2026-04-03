# Muster Deployment Checklist

This checklist must be completed for every deployment to ensure safe and successful releases.

## Pre-Deployment Checklist

### 1. Release Preparation (24 hours before)

- [ ] **Version Number**
  - [ ] Version follows semantic versioning (MAJOR.MINOR.PATCH)
  - [ ] Version is tagged in git
  - [ ] Release notes are written

- [ ] **Code Quality**
  - [ ] All tests passing (CI/CD green)
  - [ ] Code review completed
  - [ ] No critical/high security vulnerabilities
  - [ ] Lint checks passing

- [ ] **Documentation**
  - [ ] CHANGELOG.md updated
  - [ ] API documentation updated (if applicable)
  - [ ] Migration guide written (if breaking changes)

- [ ] **Stakeholder Communication**
  - [ ] Deployment scheduled with team
  - [ ] On-call engineer notified
  - [ ] Customer communication prepared (if applicable)

### 2. Environment Preparation (2 hours before)

- [ ] **Database**
  - [ ] Migration scripts tested in staging
  - [ ] Database backup completed
  - [ ] Rollback plan documented

- [ ] **Infrastructure**
  - [ ] Resource capacity verified
  - [ ] Network connectivity checked
  - [ ] Secrets and configs validated

- [ ] **Monitoring**
  - [ ] Baseline metrics captured
  - [ ] Alert thresholds reviewed
  - [ ] Dashboards accessible

### 3. Final Checks (30 minutes before)

- [ ] **Staging Validation**
  - [ ] Staging deployment successful
  - [ ] Smoke tests passing in staging
  - [ ] No critical errors in staging logs

- [ ] **Production Readiness**
  - [ ] Production config validated
  - [ ] Secrets are correct
  - [ ] Database connection verified

## Deployment Checklist

### 1. Pre-Deployment

- [ ] Announce deployment start in team channel
- [ ] Enable heightened monitoring
- [ ] Verify on-call engineer availability
- [ ] Open deployment ticket/change record

### 2. Database Operations (if applicable)

- [ ] Create database backup
- [ ] Run migrations (if required)
- [ ] Verify migration success
- [ ] Test database connectivity

### 3. Application Deployment

- [ ] Execute deployment script
- [ ] Monitor rollout progress
- [ ] Verify pods are healthy
- [ ] Check for errors in logs

### 4. Post-Deployment Verification

- [ ] Health checks passing
- [ ] Service endpoints responding
- [ ] Database connections working
- [ ] External integrations functional

## Post-Deployment Checklist

### Immediate (0-15 minutes)

- [ ] **Health Verification**
  - [ ] All pods running
  - [ ] No crash loops
  - [ ] Health endpoint responding
  - [ ] Error rate normal

- [ ] **Smoke Tests**
  - [ ] Core functionality working
  - [ ] API endpoints responding
  - [ ] Authentication working
  - [ ] Database queries executing

### Short-term (15-60 minutes)

- [ ] **Metrics Review**
  - [ ] Error rate < 0.1%
  - [ ] Response time P95 < 200ms
  - [ ] CPU usage normal
  - [ ] Memory usage normal
  - [ ] No new alerts

- [ ] **Log Analysis**
  - [ ] No critical errors
  - [ ] No unusual warnings
  - [ ] Traffic patterns normal

### Medium-term (1-4 hours)

- [ ] **Stability Verification**
  - [ ] System stable
  - [ ] No memory leaks
  - [ ] No performance degradation
  - [ ] All integrations working

- [ ] **User Experience**
  - [ ] Customer workflows functional
  - [ ] No increase in support tickets
  - [ ] User-facing features working

### Long-term (4-24 hours)

- [ ] **Final Validation**
  - [ ] Metrics stable
  - [ ] Logs normal
  - [ ] No customer issues reported
  - [ ] All systems operational

## Environment-Specific Checklist

### Development

- [ ] Database migrations tested
- [ ] Feature flags configured
- [ ] Debug logging enabled
- [ ] Test data loaded (if needed)

### Staging

- [ ] Production-like data loaded
- [ ] SSL certificates valid
- [ ] External service mocks configured
- [ ] Performance tests passing

### Production

- [ ] Change management ticket approved
- [ ] Database backup verified
- [ ] Rollback plan reviewed
- [ ] Communication sent to stakeholders
- [ ] On-call engineer confirmed available
- [ ] Business hours deployment (unless emergency)
- [ ] Maintenance window set (if applicable)

## Rollback Triggers

**Immediate Rollback Required:**
- [ ] Error rate > 5%
- [ ] Complete service outage
- [ ] Data corruption detected
- [ ] Security vulnerability exposed
- [ ] Database connection failures

**Consider Rollback:**
- [ ] Error rate > 1% and increasing
- [ ] P95 latency > 500ms
- [ ] Memory usage > 90%
- [ ] CPU throttling
- [ ] Customer complaints increasing

## Post-Rollback Actions

If rollback occurs:

1. [ ] Document rollback reason
2. [ ] Restore database from backup (if needed)
3. [ ] Notify stakeholders
4. [ ] Update deployment ticket
5. [ ] Schedule post-mortem
6. [ ] Fix issues in next release

## Sign-Off

**Deployer:**
- Name: ___________________
- Date: ___________________
- Signature: _______________

**Reviewer:**
- Name: ___________________
- Date: ___________________
- Signature: _______________

**Approval (Production only):**
- Name: ___________________
- Date: ___________________
- Signature: _______________

## Notes

<!-- Use this space for any additional notes or observations during deployment -->

---

**References:**
- [OPS-DEPLOYMENT.md](../../specs/OPS-DEPLOYMENT.md)
- [deploy-prod.md](./deploy-prod.md)
- [rollback.sh](../scripts/rollback.sh)
