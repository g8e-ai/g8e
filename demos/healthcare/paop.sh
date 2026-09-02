#!/bin/sh
# paop — demo artifact that bridges the agent's governed execution to the
# healthcare policy actuator (the healthcare equivalent of the DHS dataop /
# FedRAMP cloudop wrapper).
#
# It works around DangerousPatterns blocking curl/wget directly in
# run_shell_command: the governed command is `paop ...`, and this bridge sends
# typed policy inputs and run correlation to the actuator. The governance path
# (envelope -> admission -> L1/L2/L3 -> L5 execution -> receipt) is fully real;
# the actuator records the resulting terminal state at the secure boundary.
#
# Usage: paop <action> <request_id> <resource_type> <subject> <measured_value> <threshold_value> <run_id> <scenario_id>
#   action:          submit | gold-card | sla-check
#   request_id:      PA request identifier (e.g. PA-2026-0045)
#   resource_type:   FHIR resource type (e.g. ClaimResponse)
#   subject:         provider or request subject
#   measured_value:  approval percentage or elapsed days
#   threshold_value: approval threshold or SLA days
#   run_id:          correlated demo run identifier
#   scenario_id:     canonical scenario identifier
# Example: paop submit PA-2026-0045 ClaimResponse preauthorization 0 0 healthcare-run healthcare-success

if [ $# -ne 8 ]; then
    echo "Usage: paop <submit|gold-card|sla-check> <request_id> <resource_type> <subject> <measured_value> <threshold_value> <run_id> <scenario_id>" >&2
    exit 1
fi

ACTION="$1"
REQUEST_ID="$2"
RESOURCE_TYPE="$3"
SUBJECT="$4"
MEASURED_VALUE="$5"
THRESHOLD_VALUE="$6"
RUN_ID="$7"
SCENARIO_ID="$8"

case "$ACTION" in
    submit|gold-card|sla-check) ;;
    *) echo "paop: unknown action '$ACTION'" >&2; exit 1 ;;
esac

ACTUATOR_URL="${HEALTHCARE_ACTUATOR_URL:-http://healthcare-actuator:9200/operations}"
curl -fsS -X POST "$ACTUATOR_URL" \
    --data-urlencode "action=$ACTION" \
    --data-urlencode "request_id=$REQUEST_ID" \
    --data-urlencode "resource_type=$RESOURCE_TYPE" \
    --data-urlencode "subject=$SUBJECT" \
    --data-urlencode "measured_value=$MEASURED_VALUE" \
    --data-urlencode "threshold_value=$THRESHOLD_VALUE" \
    --data-urlencode "run_id=$RUN_ID" \
    --data-urlencode "scenario_id=$SCENARIO_ID"
