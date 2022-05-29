#!/usr/bin/env bash


### View backups.
###
### Usage:
###     $ docker-compose -f <environment>.yml (exec |run --rm) postgres backups


set -o errexit
set -o pipefail
set -o nounset


working_dir="$(dirname ${0})"
source "${working_dir}/_sourced/constants.sh"
source "${working_dir}/_sourced/messages.sh"


message_welcome "These are the backups you have got:"

ls -lht "${BACKUP_DIR_PATH}"
