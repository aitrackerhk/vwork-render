#!/bin/bash

# Configuration
SUBDOMAIN="admin40"
ENV_FILE="/opt/vwork/.env"

# Check if .env file exists
if [ ! -f "$ENV_FILE" ]; then
    echo "Error: .env file not found at $ENV_FILE"
    echo "Please ensure you are running this on the production server where vwork is installed."
    exit 1
fi

# Load environment variables from .env
# Using set -a to automatically export variables
set -a
source <(grep -v '^#' "$ENV_FILE" | sed -E 's/^([^=]+)=(.*)$/\1="\2"/')
set +a

# Verify required variables
if [ -z "$DB_HOST" ] || [ -z "$DB_USER" ] || [ -z "$DB_NAME" ]; then
    echo "Error: Database configuration missing in .env (DB_HOST, DB_USER, DB_NAME)"
    exit 1
fi

# Set PGPASSWORD for psql if DB_PASSWORD is set
if [ -n "$DB_PASSWORD" ]; then
    export PGPASSWORD="$DB_PASSWORD"
fi

echo "Connecting to database $DB_NAME on $DB_HOST as $DB_USER..."
echo "Updating expiration for tenant: $SUBDOMAIN"

# Execute SQL updates
# 1. Update trial_expires_at in tenants table
# 2. Update current_period_end in subscriptions table (if exists)

psql -h "$DB_HOST" -U "$DB_USER" -d "$DB_NAME" -p "$DB_PORT" -c "
BEGIN;

-- Update Tenant Trial Expiration
UPDATE tenants 
SET trial_expires_at = NOW() + INTERVAL '1 month' 
WHERE subdomain = '$SUBDOMAIN';

-- Update Subscription Period End (only if a subscription exists)
UPDATE subscriptions 
SET current_period_end = NOW() + INTERVAL '1 month' 
WHERE tenant_id = (SELECT id FROM tenants WHERE subdomain = '$SUBDOMAIN');

COMMIT;
"

if [ $? -eq 0 ]; then
    echo "Successfully updated expiration date to $(date -d "+1 month" +%Y-%m-%d)"
else
    echo "Update failed."
fi

# Clear PGPASSWORD
unset PGPASSWORD
