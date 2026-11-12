#!/usr/bin/env bash
set -e

BACKUP_PATH="/backup"

# Make sure that required directories exist
mkdir -p "${BACKUP_PATH}"
mkdir -p "/etc/crontabs"
chown git:git /backup
chmod 2770 /backup

# [string] BACKUP_INTERVAL       Period expression
# [string] BACKUP_RETENTION      Period expression(Deprecated for forward compatibility)
# [int]    BACKUP_RETENTION_NUM  Number of surviving backup items
if [ -z "${BACKUP_INTERVAL}" ]; then
	echo "Backup disabled: BACKUP_INTERVAL has not been found" 1>&2
	exit 1
fi

if [ -z "${BACKUP_RETENTION}" ]; then
	echo "Backup retention period is not defined, default to 7 days" 1>&2
	BACKUP_RETENTION='7d'
fi

if [ -z "${BACKUP_RETENTION_NUM}" ]; then
	echo "Backup retention number is not defined, will calculate its value later" 1>&2
else
    echo "Backup retention number is given: ${BACKUP_RETENTION_NUM}, will check its value later" 1>&2
fi

# Parse BACKUP_INTERVAL environment variable and generate appropriate cron expression. Backup cron task will be run as scheduled.
# Expected format: nu (n - number, u - unit) (eg. 3d means 3 days)
# Supported units: h - hours, d - days, M - months
parse_generate_cron_expression() {
	CRON_EXPR_MINUTES="*"
	CRON_EXPR_HOURS="*"
	CRON_EXPR_DAYS="*"
	CRON_EXPR_MONTHS="*"

    # shellcheck disable=SC2001
	TIME_INTERVAL=$(echo "${BACKUP_INTERVAL}" | sed -e 's/[hdM]$//')
    # shellcheck disable=SC2001
	TIME_UNIT=$(echo "${BACKUP_INTERVAL}" | sed -e 's/^[0-9]\+//')

	if [ "${TIME_UNIT}" = "h" ]; then
		if [ ! "${TIME_INTERVAL}" -le 23 ]; then
			echo "Parse error: Time unit 'h' (hour) cannot be greater than 23" 1>&2
			exit 1
		fi

		CRON_EXPR_MINUTES=0
		CRON_EXPR_HOURS="*/${TIME_INTERVAL}"
	elif [ "${TIME_UNIT}" = "d" ]; then
		if [ ! "${TIME_INTERVAL}" -le 30 ]; then
			echo "Parse error: Time unit 'd' (day) cannot be greater than 30" 1>&2
			exit 1
		fi

		CRON_EXPR_MINUTES=0
		CRON_EXPR_HOURS=0
		CRON_EXPR_DAYS="*/${TIME_INTERVAL}"
	elif [ "${TIME_UNIT}" = "M" ]; then
		if [ ! "${TIME_INTERVAL}" -le 12 ]; then
			echo "Parse error: Time unit 'M' (month) cannot be greater than 12" 1>&2
			exit 1
		fi

		CRON_EXPR_MINUTES=0
		CRON_EXPR_HOURS=0
		CRON_EXPR_DAYS="1"
		CRON_EXPR_MONTHS="*/${TIME_INTERVAL}"
	else
		echo "Parse error: BACKUP_INTERVAL expression is invalid" 1>&2
		exit 1
	fi

	echo "${CRON_EXPR_MINUTES} ${CRON_EXPR_HOURS} ${CRON_EXPR_DAYS} ${CRON_EXPR_MONTHS} *"
}

# Parse BACKUP_RETENTION environment variable and generate appropriate find command expression.
# Expected format: nu (n - number, u - unit) (eg. 3d means 3 days)
# Supported units: m - minutes, d - days
parse_generate_retention_expression() {
	FIND_TIME_EXPR='mtime'

	# shellcheck disable=SC2001
	TIME_INTERVAL=$(echo "${BACKUP_RETENTION}" | sed -e 's/[mhdM]$//')
	# shellcheck disable=SC2001
	TIME_UNIT=$(echo "${BACKUP_RETENTION}" | sed -e 's/^[0-9]\+//')

	if [ "${TIME_UNIT}" = "m" ]; then
		if [ "${TIME_INTERVAL}" -le 59 ]; then
			echo "Warning: Minimal retention is 60m. Value set to 60m" 1>&2
			TIME_INTERVAL=60
		fi

		FIND_TIME_EXPR="mmin"
	elif [ "${TIME_UNIT}" = "h" ]; then
		echo "Error: Unsupported expression - Try: eg. 120m for 2 hours." 1>&2
		exit 1
	elif [ "${TIME_UNIT}" = "d" ]; then
		FIND_TIME_EXPR="mtime"
	elif [ "${TIME_UNIT}" = "M" ]; then
		echo "Error: Unsupported expression - Try: eg. 60d for 2 months." 1>&2
		exit 1
	else
		echo "Parse error: BACKUP_RETENTION expression is invalid" 1>&2
		exit 1
	fi

	echo "${FIND_TIME_EXPR} +${TIME_INTERVAL:-7}"
}

# util function: convert expression to minutes, ignore input check
# Expected format: nu (n - number, u - unit) (eg. 3d means 3 days)
# Supported units: m - minutes, h - hours, d - days, M - months
parse_expression_minutes() {
	TIME_EXPRESSION="${1:-7d}"
	# shellcheck disable=SC2001
	TIME_INTERVAL=$(echo "${TIME_EXPRESSION}" | sed -e 's/[mhdM]$//')
	# shellcheck disable=SC2001
	TIME_UNIT=$(echo "${TIME_EXPRESSION}" | sed -e 's/^[0-9]\+//')

	MINUTE_MULTIPLE="1"
	if   [ "${TIME_UNIT}" = "m" ]; then
		MINUTE_MULTIPLE="1"
	elif [ "${TIME_UNIT}" = "h" ]; then
		MINUTE_MULTIPLE="60"
	elif [ "${TIME_UNIT}" = "d" ]; then
		MINUTE_MULTIPLE="1440"
	elif [ "${TIME_UNIT}" = "M" ]; then
		MINUTE_MULTIPLE="43200"
	else
		echo "Parse error: expression is invalid: ${TIME_EXPRESSION}" 1>&2
		exit 1
	fi

	echo "$(( TIME_INTERVAL * MINUTE_MULTIPLE ))"

}

# Giving BACKUP_INTERVAL and BACKUP_RETENTION, calculate the value of BACKUP_RETENTION_NUM we expected.
# Using BACKUP_RETENTION_NUM directly if BACKUP_RETENTION_NUM is given, ignoring other inputs.
calc_surviving_backups_number() {
	# shellcheck disable=SC2086  # Since the input of these two variables has been verified as valid above
	CALC_BACKUP_RETENTION_NUM="$(( $(parse_expression_minutes ${BACKUP_RETENTION}) / $(parse_expression_minutes ${BACKUP_INTERVAL}) ))"
	
	echo "${BACKUP_RETENTION_NUM:-${CALC_BACKUP_RETENTION_NUM}}"
}

add_backup_cronjob() {
	CRONTAB_USER="${1:-git}"
	CRONTAB_FILE="/etc/crontabs/${CRONTAB_USER}"
	CRONJOB_EXPRESSION="${2:-}"
	CRONJOB_EXECUTOR="${3:-}"
	CRONJOB_EXECUTOR_ARGUMENTS="${4:-}"
  CRONJOB_TASK="${CRONJOB_EXPRESSION} /bin/sh ${CRONJOB_EXECUTOR} ${CRONJOB_EXECUTOR_ARGUMENTS}"

	if [ -f "${CRONTAB_FILE}" ]; then
		CRONJOB_EXECUTOR_COUNT=$(grep -c "${CRONJOB_EXECUTOR}" "${CRONTAB_FILE}" || exit 0)
		if [ "${CRONJOB_EXECUTOR_COUNT}" != "0" ]; then
			echo "Cron job already exists for ${CRONJOB_EXECUTOR}. Updating existing." 1>&2
			CRONJOB_TASK=$(echo "{CRONJOB_TASK}" | sed 's/\//\\\//g' )
			CRONJOB_EXECUTOR=$(echo "{CRONJOB_EXECUTOR}" | sed 's/\//\\\//g' )
			sed -i "/${CRONJOB_EXECUTOR}/c\\${CRONJOB_TASK}" "${CRONTAB_FILE}"
			return 0
		fi
	fi

	# Finally append new line with cron task expression
	echo "${CRONJOB_TASK}" >>"${CRONTAB_FILE}"
}

CRONTAB_USER=$(awk -v val="${PUID}" -F ":" '$3==val{print $1}' /etc/passwd)

# ================================================================================
# Keep these lines for the purpose of checking input parameters
# ================================================================================
# Up to this point, it was desirable that interpreter handles the command errors and halts execution upon any error.
# From now, we handle the errors our self.
set +e
RETENTION_EXPRESSION="$(parse_generate_retention_expression)"

if [ -z "${RETENTION_EXPRESSION}" ]; then
	echo "Couldn't generate backup retention expression. Aborting backup setup" 1>&2
	exit 1
fi
# ================================================================================

RETENTION_NUM="$(calc_surviving_backups_number)"

if echo "${RETENTION_NUM}" | grep -qE '^[0-9]+$'; then
	echo "use RETENTION_NUM: ${RETENTION_NUM}"
else
	echo "RETENTION_NUM(${RETENTION_NUM}) is not a valid number, please check the input:" 1>&2
	echo "input vars: BACKUP_INTERVAL=${BACKUP_INTERVAL} BACKUP_RETENTION=${BACKUP_RETENTION} BACKUP_RETENTION_NUM=${BACKUP_RETENTION_NUM}" 1>&2
	exit 1
fi

# Backup rotator cron will run every 5 minutes
add_backup_cronjob "${CRONTAB_USER}" "*/5 * * * *" "/app/gogs/docker/runtime/backup-rotator.sh" "'${BACKUP_PATH}' '${RETENTION_NUM}'"
add_backup_cronjob "${CRONTAB_USER}" "$(parse_generate_cron_expression)" "/app/gogs/docker/runtime/backup-job.sh" "'${BACKUP_PATH}'"
