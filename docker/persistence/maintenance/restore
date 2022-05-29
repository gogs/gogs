#!/usr/bin/env bash


### Restore database from a backup.
###
### Parameters:
###     <1> filename of an existing backup.
###
### Usage:
###     $ docker-compose -f <environment>.yml (exec |run --rm) postgres restore <1>


set -o errexit
set -o pipefail
set -o nounset


working_dir="$(dirname ${0})"
source "${working_dir}/_sourced/constants.sh"
source "${working_dir}/_sourced/messages.sh"


if [[ -z ${1+x} ]]; then
    message_error "Backup filename is not specified yet it is a required parameter. Make sure you provide one and try again."
    exit 1
fi
backup_filename="${BACKUP_DIR_PATH}/${1}"
if [[ ! -f "${backup_filename}" ]]; then
    message_error "No backup with the specified filename found. Check out the 'backups' maintenance script output to see if there is one and try again."
    exit 1
fi

message_welcome "Restoring the '${POSTGRES_DB}' database from the '${backup_filename}' backup..."

if [[ "${POSTGRES_USER}" == "postgres" ]]; then
    message_error "Restoring as 'postgres' user is not supported. Assign 'POSTGRES_USER' env with another one and try again."
    exit 1
fi

export PGHOST="${POSTGRES_HOST}"
export PGPORT="${POSTGRES_PORT}"
export PGUSER="${POSTGRES_USER}"
export PGPASSWORD="${POSTGRES_PASSWORD}"
export PGDATABASE="${POSTGRES_DB}"

message_info "Dropping the database..."
dropdb "${PGDATABASE}"

message_info "Creating a new database..."
createdb --owner="${POSTGRES_USER}"

message_info "Applying the backup to the new database..."
gunzip -c "${backup_filename}" | psql "${POSTGRES_DB}"

message_success "The '${POSTGRES_DB}' database has been restored from the '${backup_filename}' backup."
