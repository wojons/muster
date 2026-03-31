-- +goose Up
-- +goose StatementBegin

-- HIPAA RBAC Roles table
CREATE TABLE IF NOT EXISTS hipaa_roles (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    description TEXT,
    parent_role_id TEXT REFERENCES hipaa_roles(id),
    minimum_necessary BOOLEAN DEFAULT TRUE,
    created_by TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, name)
);

CREATE INDEX idx_hipaa_roles_tenant ON hipaa_roles(tenant_id);
CREATE INDEX idx_hipaa_roles_type ON hipaa_roles(type);

-- HIPAA RBAC Permissions table
CREATE TABLE IF NOT EXISTS hipaa_permissions (
    id TEXT PRIMARY KEY,
    role_id TEXT NOT NULL REFERENCES hipaa_roles(id) ON DELETE CASCADE,
    resource TEXT NOT NULL,
    action TEXT NOT NULL,
    scope TEXT DEFAULT 'default',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(role_id, resource, action, scope)
);

CREATE INDEX idx_hipaa_permissions_role ON hipaa_permissions(role_id);

-- HIPAA Users table
CREATE TABLE IF NOT EXISTS hipaa_users (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    username TEXT NOT NULL,
    email TEXT NOT NULL,
    employee_id TEXT, -- National Provider Identifier (NPI) or internal ID
    department TEXT,
    job_title TEXT,
    status TEXT DEFAULT 'active' CHECK(status IN ('active', 'suspended', 'terminated')),
    last_login_at TIMESTAMP,
    last_password_change TIMESTAMP,
    failed_login_attempts INTEGER DEFAULT 0,
    account_locked_until TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, username),
    UNIQUE(tenant_id, email),
    UNIQUE(tenant_id, employee_id)
);

CREATE INDEX idx_hipaa_users_tenant ON hipaa_users(tenant_id);
CREATE INDEX idx_hipaa_users_status ON hipaa_users(status);

-- HIPAA User-Role Assignments table
CREATE TABLE IF NOT EXISTS hipaa_user_roles (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES hipaa_users(id) ON DELETE CASCADE,
    role_id TEXT NOT NULL REFERENCES hipaa_roles(id) ON DELETE CASCADE,
    tenant_id TEXT NOT NULL,
    assigned_by TEXT NOT NULL,
    assigned_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP,
    is_active BOOLEAN DEFAULT TRUE,
    UNIQUE(user_id, role_id, tenant_id)
);

CREATE INDEX idx_hipaa_user_roles_user ON hipaa_user_roles(user_id);
CREATE INDEX idx_hipaa_user_roles_role ON hipaa_user_roles(role_id);
CREATE INDEX idx_hipaa_user_roles_tenant ON hipaa_user_roles(tenant_id);

-- Break-Glass Authorizations table
CREATE TABLE IF NOT EXISTS hipaa_break_glass (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES hipaa_users(id),
    tenant_id TEXT NOT NULL,
    role_id TEXT NOT NULL REFERENCES hipaa_roles(id),
    reason TEXT NOT NULL,
    justification TEXT NOT NULL,
    authorized_by TEXT,
    authorized_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL,
    reviewed_by TEXT,
    reviewed_at TIMESTAMP,
    review_status TEXT DEFAULT 'pending' CHECK(review_status IN ('pending', 'approved', 'rejected')),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_hipaa_break_glass_user ON hipaa_break_glass(user_id);
CREATE INDEX idx_hipaa_break_glass_tenant ON hipaa_break_glass(tenant_id);
CREATE INDEX idx_hipaa_break_glass_status ON hipaa_break_glass(review_status);
CREATE INDEX idx_hipaa_break_glass_expires ON hipaa_break_glass(expires_at);

-- PHI Resources table
CREATE TABLE IF NOT EXISTS hipaa_phi_resources (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    patient_id TEXT NOT NULL,
    classification TEXT DEFAULT 'normal' CHECK(classification IN ('normal', 'sensitive', 'restricted')),
    access_policy_id TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, resource_type, resource_id)
);

CREATE INDEX idx_hipaa_phi_resources_tenant ON hipaa_phi_resources(tenant_id);
CREATE INDEX idx_hipaa_phi_resources_patient ON hipaa_phi_resources(patient_id);
CREATE INDEX idx_hipaa_phi_resources_classification ON hipaa_phi_resources(classification);

-- Access Logs table (HIPAA audit requirement)
CREATE TABLE IF NOT EXISTS hipaa_access_logs (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    role_id TEXT,
    resource_type TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    action TEXT NOT NULL,
    result TEXT NOT NULL CHECK(result IN ('allowed', 'denied')),
    reason TEXT,
    break_glass_used BOOLEAN DEFAULT FALSE,
    break_glass_id TEXT REFERENCES hipaa_break_glass(id),
    purpose_of_use TEXT NOT NULL,
    access_type TEXT NOT NULL,
    client_ip TEXT,
    user_agent TEXT,
    session_id TEXT,
    timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_hipaa_access_logs_tenant ON hipaa_access_logs(tenant_id);
CREATE INDEX idx_hipaa_access_logs_user ON hipaa_access_logs(user_id);
CREATE INDEX idx_hipaa_access_logs_resource ON hipaa_access_logs(resource_type, resource_id);
CREATE INDEX idx_hipaa_access_logs_timestamp ON hipaa_access_logs(timestamp);
CREATE INDEX idx_hipaa_access_logs_break_glass ON hipaa_access_logs(break_glass_used);

-- Insert default HIPAA roles
INSERT INTO hipaa_roles (id, tenant_id, name, type, description, minimum_necessary, created_by) VALUES
    ('role_physician_default', 'default', 'Physician', 'physician', 'Licensed physician with full patient access', TRUE, 'system'),
    ('role_nurse_default', 'default', 'Nurse', 'nurse', 'Registered nurse with patient care access', TRUE, 'system'),
    ('role_admin_default', 'default', 'Administrator', 'administrator', 'System administrator with limited PHI access', TRUE, 'system'),
    ('role_billing_default', 'default', 'Billing', 'billing', 'Billing staff with insurance claim access', TRUE, 'system'),
    ('role_researcher_default', 'default', 'Researcher', 'researcher', 'Researcher with de-identified data access', TRUE, 'system'),
    ('role_pharmacist_default', 'default', 'Pharmacist', 'pharmacist', 'Licensed pharmacist with prescription access', TRUE, 'system'),
    ('role_lab_default', 'default', 'Lab Technician', 'lab_technician', 'Lab technician with lab result access', TRUE, 'system'),
    ('role_receptionist_default', 'default', 'Receptionist', 'receptionist', 'Front desk with appointment access only', TRUE, 'system'),
    ('role_compliance_default', 'default', 'Compliance Officer', 'compliance_officer', 'HIPAA compliance officer with audit access', TRUE, 'system'),
    ('role_emergency_default', 'default', 'Emergency', 'emergency', 'Emergency break-glass role (temporary)', FALSE, 'system');

-- Insert default permissions for Physician role
INSERT INTO hipaa_permissions (id, role_id, resource, action, scope) VALUES
    ('perm_phys_phi_read', 'role_physician_default', 'phi', 'read', 'own_patients'),
    ('perm_phys_phi_write', 'role_physician_default', 'phi', 'write', 'own_patients'),
    ('perm_phys_phi_update', 'role_physician_default', 'phi', 'update', 'own_patients'),
    ('perm_phys_records_read', 'role_physician_default', 'patient_records', 'read', 'own_patients'),
    ('perm_phys_records_write', 'role_physician_default', 'patient_records', 'write', 'own_patients'),
    ('perm_phys_prescriptions_read', 'role_physician_default', 'prescriptions', 'read', 'own_patients'),
    ('perm_phys_prescriptions_write', 'role_physician_default', 'prescriptions', 'write', 'own_patients'),
    ('perm_phys_labs_read', 'role_physician_default', 'lab_results', 'read', 'own_patients'),
    ('perm_phys_labs_write', 'role_physician_default', 'lab_results', 'write', 'own_patients'),
    ('perm_phys_audit_read', 'role_physician_default', 'audit', 'read', 'own_patients');

-- Insert default permissions for Nurse role
INSERT INTO hipaa_permissions (id, role_id, resource, action, scope) VALUES
    ('perm_nurse_phi_read', 'role_nurse_default', 'phi', 'read', 'assigned'),
    ('perm_nurse_phi_update', 'role_nurse_default', 'phi', 'update', 'assigned'),
    ('perm_nurse_records_read', 'role_nurse_default', 'patient_records', 'read', 'assigned'),
    ('perm_nurse_records_update', 'role_nurse_default', 'patient_records', 'update', 'assigned'),
    ('perm_nurse_prescriptions_read', 'role_nurse_default', 'prescriptions', 'read', 'assigned'),
    ('perm_nurse_labs_read', 'role_nurse_default', 'lab_results', 'read', 'assigned');

-- Insert default permissions for Administrator role
INSERT INTO hipaa_permissions (id, role_id, resource, action, scope) VALUES
    ('perm_admin_users_read', 'role_admin_default', 'users', 'read', 'organization'),
    ('perm_admin_users_write', 'role_admin_default', 'users', 'write', 'organization'),
    ('perm_admin_audit_read', 'role_admin_default', 'audit', 'read', 'organization'),
    ('perm_admin_config_read', 'role_admin_default', 'config', 'read', 'organization'),
    ('perm_admin_config_write', 'role_admin_default', 'config', 'write', 'organization');

-- Insert default permissions for Billing role
INSERT INTO hipaa_permissions (id, role_id, resource, action, scope) VALUES
    ('perm_bill_claims_read', 'role_billing_default', 'insurance_claims', 'read', 'organization'),
    ('perm_bill_claims_write', 'role_billing_default', 'insurance_claims', 'write', 'organization'),
    ('perm_bill_records_read', 'role_billing_default', 'patient_records', 'read', 'assigned'),
    ('perm_bill_audit_read', 'role_billing_default', 'audit', 'read', 'organization');

-- Insert default permissions for Researcher role (de-identified only)
INSERT INTO hipaa_permissions (id, role_id, resource, action, scope) VALUES
    ('perm_research_phi_read', 'role_researcher_default', 'phi', 'read', 'deidentified'),
    ('perm_research_records_read', 'role_researcher_default', 'patient_records', 'read', 'deidentified'),
    ('perm_research_audit_read', 'role_researcher_default', 'audit', 'read', 'own');

-- Insert default permissions for Pharmacist role
INSERT INTO hipaa_permissions (id, role_id, resource, action, scope) VALUES
    ('perm_pharm_prescriptions_read', 'role_pharmacist_default', 'prescriptions', 'read', 'organization'),
    ('perm_pharm_prescriptions_write', 'role_pharmacist_default', 'prescriptions', 'write', 'organization'),
    ('perm_pharm_records_read', 'role_pharmacist_default', 'patient_records', 'read', 'assigned'),
    ('perm_pharm_audit_read', 'role_pharmacist_default', 'audit', 'read', 'own');

-- Insert default permissions for Lab Technician role
INSERT INTO hipaa_permissions (id, role_id, resource, action, scope) VALUES
    ('perm_lab_results_read', 'role_lab_default', 'lab_results', 'read', 'assigned'),
    ('perm_lab_results_write', 'role_lab_default', 'lab_results', 'write', 'assigned'),
    ('perm_lab_records_read', 'role_lab_default', 'patient_records', 'read', 'assigned'),
    ('perm_lab_audit_read', 'role_lab_default', 'audit', 'read', 'own');

-- Insert default permissions for Receptionist role
INSERT INTO hipaa_permissions (id, role_id, resource, action, scope) VALUES
    ('perm_recept_appointments_read', 'role_receptionist_default', 'appointments', 'read', 'organization'),
    ('perm_recept_appointments_write', 'role_receptionist_default', 'appointments', 'write', 'organization'),
    ('perm_recept_records_read', 'role_receptionist_default', 'patient_records', 'read', 'demographics_only');

-- Insert default permissions for Compliance Officer role
INSERT INTO hipaa_permissions (id, role_id, resource, action, scope) VALUES
    ('perm_comp_audit_read', 'role_compliance_default', 'audit', 'read', 'all'),
    ('perm_comp_audit_export', 'role_compliance_default', 'audit', 'export', 'all'),
    ('perm_comp_logs_read', 'role_compliance_default', 'logs', 'read', 'all'),
    ('perm_comp_users_read', 'role_compliance_default', 'users', 'read', 'all'),
    ('perm_comp_breakglass_read', 'role_compliance_default', 'break_glass', 'read', 'all'),
    ('perm_comp_breakglass_approve', 'role_compliance_default', 'break_glass', 'approve', 'all');

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS hipaa_access_logs;
DROP TABLE IF EXISTS hipaa_phi_resources;
DROP TABLE IF EXISTS hipaa_break_glass;
DROP TABLE IF EXISTS hipaa_user_roles;
DROP TABLE IF EXISTS hipaa_users;
DROP TABLE IF EXISTS hipaa_permissions;
DROP TABLE IF EXISTS hipaa_roles;

-- +goose StatementEnd
