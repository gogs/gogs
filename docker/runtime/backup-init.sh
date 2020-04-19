#!/usr/bin/env bash
set -e

BACKUP_PATH="/backup"

# Make sure that required directories exist
mkdir -p "${BACKUP_PATH}"
mkdir -p "/etc/crontabs"

# [int] BACKUP_INTERVAL   Period with prefix
# [int] BACKUP_RETENTION  Period in days
if [ -z "${BACKUP_INTERVAL}" ]; then
	echo "Backup disabled: BACKUP_INTERVAL has not been found"
	exit 1
fi

if [ -z "${BACKUP_RETENTION}" ]; then
	echo "Backup retention period not defined - default value used: 7 days"
	BACKUP_RETENTION=7
fi

# Parse BACKUP_INTERVAL environment variable and generate appropriate cron expression. Backup cron task will be run as scheduled.
# Expected format: nu (n - number, u - unit) (eg. 3d equals 3 days)
# Supported units: m - minutes, h - hours, d - days
parse_generate_cron_expression() {
	CRON_EXPR_MINUTES="*"
	CRON_EXPR_HOURS="*"
	CRON_EXPR_DAYS="*"

	TIME_INTERVAL=$(echo "${BACKUP_INTERVAL}" | sed -e 's/[mhd]$//')
	TIME_UNIT=$(echo "${BACKUP_INTERVAL}" | sed -e 's/^[0-9]\+//')

	if [ "${TIME_UNIT}" = "m" ]; then

		if [ ! "${TIME_INTERVAL}" -le 59 ]; then
			echo "Parse error: Time unit 'm' largest value is 59"
			exit 1
		fi

		CRON_EXPR_MINUTES="*/${TIME_INTERVAL}"
	elif [ "${TIME_UNIT}" = "h" ]; then

		if [ ! "${TIME_INTERVAL}" -le 23 ]; then
			echo "Parse error: Time unit 'h' largest value is 23"
			exit 1
		fi

		CRON_EXPR_MINUTES=0
		CRON_EXPR_HOURS="*/${TIME_INTERVAL}"
	elif [ "${TIME_UNIT}" = "d" ]; then

		if [ ! "${TIME_INTERVAL}" -le 30 ]; then
			echo "Parse error: Time unit 'd' largest value is 30"
			exit 1
		fi

		CRON_EXPR_MINUTES=0
		CRON_EXPR_HOURS=0
		CRON_EXPR_DAYS="*/${TIME_INTERVAL}"
	else
		echo "Parse error: BACKUP_INTERVAL expression is invalid"
		exit 1
	fi

	echo "${CRON_EXPR_MINUTES} ${CRON_EXPR_HOURS} ${CRON_EXPR_DAYS} * *"
}

add_backup_cronjob() {
	CRONTAB_USER="${1:-git}"
	CRONTAB_FILE="rootfs/etc/crontabs/${CRONTAB_USER}"
	CRONJOB_EXPRESSION="${2:-}"
	CRONJOB_EXECUTOR="${3:-}"

	if [ -f "${CRONTAB_FILE}" ]; then
		CRONJOB_EXECUTOR_COUNT=$(grep -c "${CRONJOB_EXECUTOR}" "${CRONTAB_FILE}" || exit 0)
		if [ "${CRONJOB_EXECUTOR_COUNT}" != "0" ]; then
			echo "Cron job already exists for ${CRONJOB_EXECUTOR}. Refusing to add duplicate."
			return 1
		fi
	fi

	# Finally append new line with cron task expression
	echo "${CRONJOB_EXPRESSION} ${CRONJOB_EXECUTOR} '${BACKUP_PATH}' '${BACKUP_RETENTION}'" >>"${CRONTAB_FILE}"
}

CRONTAB_USER=$(awk -v val="${PUID}" -F ":" '$3==val{print $1}' /etc/passwd)

set +e
# Backup rotator cron will run every 12 hours
add_backup_cronjob "${CRONTAB_USER}" "0 */12 * * *" "/app/gogs/docker/runtime/backup-rotator.sh"
add_backup_cronjob "${CRONTAB_USER}" "$(parse_generate_cron_expression)" "/app/gogs/docker/runtime/backup-job.sh"
