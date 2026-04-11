# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""IAM command builder for Cloud Operator Two-Role architecture.

Builds the bash commands executed on the Cloud Operator to attach, detach,
and verify AWS IAM intent policies via the Escalation Role pattern.
"""


class IamCommandBuilder:
    """Builds bash commands for AWS Two-Role IAM intent policy operations."""

    def build_attach_command(self, intent: str) -> str:
        return (
            f"set -e && "
            f"ACCOUNT_ID=$(aws sts get-caller-identity --query 'Account' --output text) && "
            f"current_arn=$(aws sts get-caller-identity --query 'Arn' --output text) && "
            f'echo "Current identity: $current_arn" && '
            f'if [[ "$current_arn" == *":root" ]]; then '
            f"  echo 'Running as root, skipping policy update' && exit 0; "
            f"fi && "
            f'if [[ "$current_arn" == *"assumed-role/"* ]] || [[ "$current_arn" == *":role/"* ]]; then '
            f"  OPERATOR_ROLE_NAME=$(echo \"$current_arn\" | sed -n 's/.*role\\/\\([^/]*\\).*/\\1/p') && "
            f"  if [ -z \"$OPERATOR_ROLE_NAME\" ]; then echo 'ERROR: Could not extract role name from ARN' && exit 1; fi && "
            f'  echo "Operator Role: $OPERATOR_ROLE_NAME" && '
            f"  ESCALATION_ROLE_NAME=$(echo \"$OPERATOR_ROLE_NAME\" | sed 's/-Operator-Role$/-Escalation-Role/') && "
            f'  if [ "$ESCALATION_ROLE_NAME" = "$OPERATOR_ROLE_NAME" ]; then '
            f'    ESCALATION_ROLE_NAME="${{OPERATOR_ROLE_NAME}}-Escalation"; '
            f"  fi && "
            f'  ESCALATION_ROLE_ARN="arn:aws:iam::$ACCOUNT_ID:role/$ESCALATION_ROLE_NAME" && '
            f'  echo "Escalation Role ARN: $ESCALATION_ROLE_ARN" && '
            f"  ROLE_PREFIX=$(echo \"$OPERATOR_ROLE_NAME\" | sed 's/-Operator-Role$//') && "
            f'  if [ "$ROLE_PREFIX" = "$OPERATOR_ROLE_NAME" ]; then '
            f'    ROLE_PREFIX="g8e-cloud-operator"; '
            f"  fi && "
            f'  INTENT_POLICY_ARN="arn:aws:iam::$ACCOUNT_ID:policy/${{ROLE_PREFIX}}-Intent-{intent}" && '
            f'  echo "Intent Policy ARN: $INTENT_POLICY_ARN" && '
            f'  EXTERNAL_ID="g8e-${{ROLE_PREFIX}}-${{ACCOUNT_ID}}" && '
            f'  echo "External ID: $EXTERNAL_ID" && '
            f"  echo 'Assuming Escalation Role...' && "
            f"  CREDS=$(aws sts assume-role "
            f'    --role-arn "$ESCALATION_ROLE_ARN" '
            f'    --role-session-name "g8e-intent-grant-{intent}" '
            f'    --external-id "$EXTERNAL_ID" '
            f"    --duration-seconds 900 "
            f"    --query 'Credentials' --output json) && "
            f'  if [ -z "$CREDS" ] || [ "$CREDS" = "null" ]; then '
            f"    echo 'ERROR: Failed to assume Escalation Role. Check IAM trust policy.' && exit 1; "
            f"  fi && "
            f"  export AWS_ACCESS_KEY_ID=$(echo $CREDS | jq -r '.AccessKeyId') && "
            f"  export AWS_SECRET_ACCESS_KEY=$(echo $CREDS | jq -r '.SecretAccessKey') && "
            f"  export AWS_SESSION_TOKEN=$(echo $CREDS | jq -r '.SessionToken') && "
            f"  echo 'Successfully assumed Escalation Role' && "
            f"  escalation_identity=$(aws sts get-caller-identity --query 'Arn' --output text) && "
            f'  echo "Now operating as: $escalation_identity" && '
            f"  echo 'Attaching intent policy to Operator Role...' && "
            f"  aws iam attach-role-policy "
            f'    --role-name "$OPERATOR_ROLE_NAME" '
            f'    --policy-arn "$INTENT_POLICY_ARN" && '
            f"  echo 'SUCCESS: Intent policy {intent} attached to Operator Role' && "
            f"  unset AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN; "
            f"else "
            f"  echo 'ERROR: Current identity is not a role (likely IAM User). Cloud Operator requires IAM Role.' && exit 1; "
            f"fi"
        )

    def build_detach_command(self, intent: str) -> str:
        return (
            f"set -e && "
            f"ACCOUNT_ID=$(aws sts get-caller-identity --query 'Account' --output text) && "
            f"current_arn=$(aws sts get-caller-identity --query 'Arn' --output text) && "
            f'echo "Current identity: $current_arn" && '
            f'if [[ "$current_arn" == *":root" ]]; then '
            f"  echo 'Running as root, skipping policy detach' && exit 0; "
            f"fi && "
            f'if [[ "$current_arn" == *"assumed-role/"* ]] || [[ "$current_arn" == *":role/"* ]]; then '
            f"  OPERATOR_ROLE_NAME=$(echo \"$current_arn\" | sed -n 's/.*role\\/\\([^/]*\\).*/\\1/p') && "
            f"  if [ -z \"$OPERATOR_ROLE_NAME\" ]; then echo 'ERROR: Could not extract role name from ARN' && exit 1; fi && "
            f'  echo "Operator Role: $OPERATOR_ROLE_NAME" && '
            f"  ESCALATION_ROLE_NAME=$(echo \"$OPERATOR_ROLE_NAME\" | sed 's/-Operator-Role$/-Escalation-Role/') && "
            f'  if [ "$ESCALATION_ROLE_NAME" = "$OPERATOR_ROLE_NAME" ]; then '
            f'    ESCALATION_ROLE_NAME="${{OPERATOR_ROLE_NAME}}-Escalation"; '
            f"  fi && "
            f'  ESCALATION_ROLE_ARN="arn:aws:iam::$ACCOUNT_ID:role/$ESCALATION_ROLE_NAME" && '
            f'  echo "Escalation Role ARN: $ESCALATION_ROLE_ARN" && '
            f"  ROLE_PREFIX=$(echo \"$OPERATOR_ROLE_NAME\" | sed 's/-Operator-Role$//') && "
            f'  if [ "$ROLE_PREFIX" = "$OPERATOR_ROLE_NAME" ]; then '
            f'    ROLE_PREFIX="g8e-cloud-operator"; '
            f"  fi && "
            f'  INTENT_POLICY_ARN="arn:aws:iam::$ACCOUNT_ID:policy/${{ROLE_PREFIX}}-Intent-{intent}" && '
            f'  echo "Intent Policy ARN: $INTENT_POLICY_ARN" && '
            f'  EXTERNAL_ID="g8e-${{ROLE_PREFIX}}-${{ACCOUNT_ID}}" && '
            f'  echo "External ID: $EXTERNAL_ID" && '
            f"  echo 'Assuming Escalation Role...' && "
            f"  CREDS=$(aws sts assume-role "
            f'    --role-arn "$ESCALATION_ROLE_ARN" '
            f'    --role-session-name "g8e-intent-revoke-{intent}" '
            f'    --external-id "$EXTERNAL_ID" '
            f"    --duration-seconds 900 "
            f"    --query 'Credentials' --output json) && "
            f'  if [ -z "$CREDS" ] || [ "$CREDS" = "null" ]; then '
            f"    echo 'ERROR: Failed to assume Escalation Role. Check IAM trust policy.' && exit 1; "
            f"  fi && "
            f"  export AWS_ACCESS_KEY_ID=$(echo $CREDS | jq -r '.AccessKeyId') && "
            f"  export AWS_SECRET_ACCESS_KEY=$(echo $CREDS | jq -r '.SecretAccessKey') && "
            f"  export AWS_SESSION_TOKEN=$(echo $CREDS | jq -r '.SessionToken') && "
            f"  echo 'Successfully assumed Escalation Role' && "
            f"  escalation_identity=$(aws sts get-caller-identity --query 'Arn' --output text) && "
            f'  echo "Now operating as: $escalation_identity" && '
            f"  echo 'Detaching intent policy from Operator Role...' && "
            f"  if aws iam detach-role-policy "
            f'    --role-name "$OPERATOR_ROLE_NAME" '
            f'    --policy-arn "$INTENT_POLICY_ARN" 2>/dev/null; then '
            f"    echo 'SUCCESS: Intent policy {intent} detached from Operator Role'; "
            f"  else "
            f"    echo 'NOTE: Policy may already be detached or does not exist'; "
            f"  fi && "
            f"  unset AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN; "
            f"else "
            f"  echo 'ERROR: Current identity is not a role (likely IAM User). Cloud Operator requires IAM Role.' && exit 1; "
            f"fi"
        )

    def build_verify_command(self, intent: str, verification_action: str) -> str:
        return (
            f"set -e && "
            f"ROLE_ARN=$(aws sts get-caller-identity --query 'Arn' --output text | "
            f"  sed 's/:sts:/:iam:/' | sed 's/assumed-role/role/' | sed 's/\\/[^\\/]*$//' ) && "
            f'echo "Verifying permission propagation for role: $ROLE_ARN" && '
            f"MAX_ATTEMPTS=10 && "
            f"ATTEMPT=0 && "
            f"while [ $ATTEMPT -lt $MAX_ATTEMPTS ]; do "
            f"  RESULT=$(aws iam simulate-principal-policy "
            f'    --policy-source-arn "$ROLE_ARN" '
            f'    --action-names "{verification_action}" '
            f"    --query 'EvaluationResults[0].EvalDecision' "
            f"    --output text 2>/dev/null || echo 'error') && "
            f'  if [ "$RESULT" = "allowed" ]; then '
            f'    echo "SUCCESS: Permission {verification_action} is now active" && '
            f"    exit 0; "
            f"  fi && "
            f"  ATTEMPT=$((ATTEMPT + 1)) && "
            f'  echo "Waiting for IAM propagation... (attempt $ATTEMPT/$MAX_ATTEMPTS, result: $RESULT)" && '
            f"  sleep 2; "
            f"done && "
            f'echo "WARNING: Permission verification timed out, proceeding anyway" && '
            f"exit 0"
        )
