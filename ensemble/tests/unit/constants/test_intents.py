# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Regression tests for Phase 10 — Intent values sourced from g8e.constants.INTENTS."""

import pytest

from g8e.constants import intent as _g8e_intent

from app.constants.intents import (
    CLOUD_INTENT_DEPENDENCIES,
    CLOUD_INTENT_QUESTIONS,
    CLOUD_INTENT_VERIFICATION_ACTIONS,
    CloudIntent,
)

pytestmark = pytest.mark.unit


class TestIntentValuesFromG8e:
    """Verify all CloudIntent values match g8e protocol constants."""

    # Map g8ee member name → g8e intent key
    _INTENT_MAP = {
        CloudIntent.EC2_DISCOVERY: "Ec2Discovery",
        CloudIntent.EC2_MANAGEMENT: "Ec2Management",
        CloudIntent.EC2_SNAPSHOT_MANAGEMENT: "Ec2SnapshotManagement",
        CloudIntent.S3_READ: "S3Read",
        CloudIntent.S3_WRITE: "S3Write",
        CloudIntent.S3_BUCKET_DISCOVERY: "S3BucketDiscovery",
        CloudIntent.S3_DELETE: "S3Delete",
        CloudIntent.TERRAFORM_STATE: "TerraformState",
        CloudIntent.CLOUDFORMATION_DEPLOYMENT: "CloudformationDeployment",
        CloudIntent.CLOUDWATCH_LOGS: "CloudwatchLogs",
        CloudIntent.SECRETS_READ: "SecretsRead",
        CloudIntent.LAMBDA_DISCOVERY: "LambdaDiscovery",
        CloudIntent.LAMBDA_INVOKE: "LambdaInvoke",
        CloudIntent.RDS_DISCOVERY: "RdsDiscovery",
        CloudIntent.RDS_MANAGEMENT: "RdsManagement",
        CloudIntent.RDS_SNAPSHOT_MANAGEMENT: "RdsSnapshotManagement",
        CloudIntent.AURORA_CLUSTER_MANAGEMENT: "AuroraClusterManagement",
        CloudIntent.AURORA_SCALING: "AuroraScaling",
        CloudIntent.AURORA_CLONING: "AuroraCloning",
        CloudIntent.AURORA_GLOBAL_DATABASE: "AuroraGlobalDatabase",
        CloudIntent.ECS_DISCOVERY: "EcsDiscovery",
        CloudIntent.ECS_MANAGEMENT: "EcsManagement",
        CloudIntent.EKS_DISCOVERY: "EksDiscovery",
        CloudIntent.VPC_DISCOVERY: "VpcDiscovery",
        CloudIntent.ELB_DISCOVERY: "ElbDiscovery",
        CloudIntent.ROUTE53_DISCOVERY: "Route53Discovery",
        CloudIntent.ROUTE53_MANAGEMENT: "Route53Management",
        CloudIntent.AUTOSCALING_DISCOVERY: "AutoscalingDiscovery",
        CloudIntent.AUTOSCALING_MANAGEMENT: "AutoscalingManagement",
        CloudIntent.CLOUDWATCH_METRICS: "CloudwatchMetrics",
        CloudIntent.SNS_DISCOVERY: "SnsDiscovery",
        CloudIntent.SNS_PUBLISH: "SnsPublish",
        CloudIntent.SQS_DISCOVERY: "SqsDiscovery",
        CloudIntent.SQS_MANAGEMENT: "SqsManagement",
        CloudIntent.EVENTBRIDGE_DISCOVERY: "EventbridgeDiscovery",
        CloudIntent.DYNAMODB_DISCOVERY: "DynamodbDiscovery",
        CloudIntent.DYNAMODB_READ: "DynamodbRead",
        CloudIntent.DYNAMODB_WRITE: "DynamodbWrite",
        CloudIntent.ELASTICACHE_DISCOVERY: "ElasticacheDiscovery",
        CloudIntent.KMS_DISCOVERY: "KmsDiscovery",
        CloudIntent.KMS_CRYPTO: "KmsCrypto",
        CloudIntent.IAM_DISCOVERY: "IamDiscovery",
        CloudIntent.ACM_DISCOVERY: "AcmDiscovery",
        CloudIntent.APIGATEWAY_DISCOVERY: "ApigatewayDiscovery",
        CloudIntent.STEPFUNCTIONS_DISCOVERY: "StepfunctionsDiscovery",
        CloudIntent.STEPFUNCTIONS_EXECUTION: "StepfunctionsExecution",
        CloudIntent.ATHENA_DISCOVERY: "AthenaDiscovery",
        CloudIntent.ATHENA_QUERY_EXECUTION: "AthenaQueryExecution",
        CloudIntent.GLUE_DISCOVERY: "GlueDiscovery",
        CloudIntent.CLOUDFRONT_DISCOVERY: "CloudfrontDiscovery",
        CloudIntent.CODEDEPLOY_DISCOVERY: "CodedeployDiscovery",
        CloudIntent.COST_EXPLORER: "CostExplorer",
    }

    @pytest.mark.parametrize(
        "member,g8e_key",
        list(_INTENT_MAP.items()),
        ids=[m.name for m in _INTENT_MAP],
    )
    def test_intent_matches_g8e(self, member: CloudIntent, g8e_key: str):
        assert member.value == _g8e_intent(g8e_key)

    def test_intent_count(self):
        assert len(list(CloudIntent)) == 52


class TestIntentMappingCompleteness:
    """Verify all 52 intents are present in all three mapping dicts."""

    def test_all_intents_in_dependencies(self):
        for member in CloudIntent:
            assert member in CLOUD_INTENT_DEPENDENCIES, (
                f"{member.name} missing from CLOUD_INTENT_DEPENDENCIES"
            )

    def test_all_intents_in_questions(self):
        for member in CloudIntent:
            assert member in CLOUD_INTENT_QUESTIONS, (
                f"{member.name} missing from CLOUD_INTENT_QUESTIONS"
            )

    def test_all_intents_in_verification_actions(self):
        for member in CloudIntent:
            assert member in CLOUD_INTENT_VERIFICATION_ACTIONS, (
                f"{member.name} missing from CLOUD_INTENT_VERIFICATION_ACTIONS"
            )

    def test_dependencies_questions_verification_same_keys(self):
        dep_keys = set(CLOUD_INTENT_DEPENDENCIES.keys())
        q_keys = set(CLOUD_INTENT_QUESTIONS.keys())
        v_keys = set(CLOUD_INTENT_VERIFICATION_ACTIONS.keys())
        assert dep_keys == q_keys == v_keys == set(CloudIntent)

    def test_dependencies_values_are_lists(self):
        for member, deps in CLOUD_INTENT_DEPENDENCIES.items():
            assert isinstance(deps, list), f"{member.name} dependencies is not a list"

    def test_questions_values_are_strings(self):
        for member, question in CLOUD_INTENT_QUESTIONS.items():
            assert isinstance(question, str), f"{member.name} question is not a string"

    def test_verification_actions_values_are_strings(self):
        for member, action in CLOUD_INTENT_VERIFICATION_ACTIONS.items():
            assert isinstance(action, str), f"{member.name} verification action is not a string"
