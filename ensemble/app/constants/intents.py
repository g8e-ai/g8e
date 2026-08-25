# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Cloud intent identifiers and mappings.

Intent values are sourced from g8e.constants.INTENTS to stay in sync with
the protocol. g8ee-specific member names are preserved for backward
compatibility with dict key references in dependency/question/verification
mappings.
"""

from enum import StrEnum

from g8e.constants import intent as _g8e_intent


class CloudIntent(StrEnum):
    """Cloud intent identifiers"""

    EC2_DISCOVERY = _g8e_intent("Ec2Discovery")
    EC2_MANAGEMENT = _g8e_intent("Ec2Management")
    EC2_SNAPSHOT_MANAGEMENT = _g8e_intent("Ec2SnapshotManagement")
    S3_READ = _g8e_intent("S3Read")
    S3_WRITE = _g8e_intent("S3Write")
    S3_BUCKET_DISCOVERY = _g8e_intent("S3BucketDiscovery")
    S3_DELETE = _g8e_intent("S3Delete")
    TERRAFORM_STATE = _g8e_intent("TerraformState")
    CLOUDFORMATION_DEPLOYMENT = _g8e_intent("CloudformationDeployment")
    CLOUDWATCH_LOGS = _g8e_intent("CloudwatchLogs")
    SECRETS_READ = _g8e_intent("SecretsRead")
    LAMBDA_DISCOVERY = _g8e_intent("LambdaDiscovery")
    LAMBDA_INVOKE = _g8e_intent("LambdaInvoke")
    RDS_DISCOVERY = _g8e_intent("RdsDiscovery")
    RDS_MANAGEMENT = _g8e_intent("RdsManagement")
    RDS_SNAPSHOT_MANAGEMENT = _g8e_intent("RdsSnapshotManagement")
    AURORA_CLUSTER_MANAGEMENT = _g8e_intent("AuroraClusterManagement")
    AURORA_SCALING = _g8e_intent("AuroraScaling")
    AURORA_CLONING = _g8e_intent("AuroraCloning")
    AURORA_GLOBAL_DATABASE = _g8e_intent("AuroraGlobalDatabase")
    ECS_DISCOVERY = _g8e_intent("EcsDiscovery")
    ECS_MANAGEMENT = _g8e_intent("EcsManagement")
    EKS_DISCOVERY = _g8e_intent("EksDiscovery")
    VPC_DISCOVERY = _g8e_intent("VpcDiscovery")
    ELB_DISCOVERY = _g8e_intent("ElbDiscovery")
    ROUTE53_DISCOVERY = _g8e_intent("Route53Discovery")
    ROUTE53_MANAGEMENT = _g8e_intent("Route53Management")
    AUTOSCALING_DISCOVERY = _g8e_intent("AutoscalingDiscovery")
    AUTOSCALING_MANAGEMENT = _g8e_intent("AutoscalingManagement")
    CLOUDWATCH_METRICS = _g8e_intent("CloudwatchMetrics")
    SNS_DISCOVERY = _g8e_intent("SnsDiscovery")
    SNS_PUBLISH = _g8e_intent("SnsPublish")
    SQS_DISCOVERY = _g8e_intent("SqsDiscovery")
    SQS_MANAGEMENT = _g8e_intent("SqsManagement")
    EVENTBRIDGE_DISCOVERY = _g8e_intent("EventbridgeDiscovery")
    DYNAMODB_DISCOVERY = _g8e_intent("DynamodbDiscovery")
    DYNAMODB_READ = _g8e_intent("DynamodbRead")
    DYNAMODB_WRITE = _g8e_intent("DynamodbWrite")
    ELASTICACHE_DISCOVERY = _g8e_intent("ElasticacheDiscovery")
    KMS_DISCOVERY = _g8e_intent("KmsDiscovery")
    KMS_CRYPTO = _g8e_intent("KmsCrypto")
    IAM_DISCOVERY = _g8e_intent("IamDiscovery")
    ACM_DISCOVERY = _g8e_intent("AcmDiscovery")
    APIGATEWAY_DISCOVERY = _g8e_intent("ApigatewayDiscovery")
    STEPFUNCTIONS_DISCOVERY = _g8e_intent("StepfunctionsDiscovery")
    STEPFUNCTIONS_EXECUTION = _g8e_intent("StepfunctionsExecution")
    ATHENA_DISCOVERY = _g8e_intent("AthenaDiscovery")
    ATHENA_QUERY_EXECUTION = _g8e_intent("AthenaQueryExecution")
    GLUE_DISCOVERY = _g8e_intent("GlueDiscovery")
    CLOUDFRONT_DISCOVERY = _g8e_intent("CloudfrontDiscovery")
    CODEDEPLOY_DISCOVERY = _g8e_intent("CodedeployDiscovery")
    COST_EXPLORER = _g8e_intent("CostExplorer")


# Mapping of intent to its dependencies
CLOUD_INTENT_DEPENDENCIES = {
    CloudIntent.EC2_DISCOVERY: [],
    CloudIntent.EC2_MANAGEMENT: ["ec2_discovery"],
    CloudIntent.EC2_SNAPSHOT_MANAGEMENT: ["ec2_discovery"],
    CloudIntent.S3_READ: [],
    CloudIntent.S3_WRITE: ["s3_read"],
    CloudIntent.S3_BUCKET_DISCOVERY: [],
    CloudIntent.S3_DELETE: ["s3_read"],
    CloudIntent.TERRAFORM_STATE: [],
    CloudIntent.CLOUDFORMATION_DEPLOYMENT: [],
    CloudIntent.CLOUDWATCH_LOGS: [],
    CloudIntent.SECRETS_READ: [],
    CloudIntent.LAMBDA_DISCOVERY: [],
    CloudIntent.LAMBDA_INVOKE: ["lambda_discovery"],
    CloudIntent.RDS_DISCOVERY: [],
    CloudIntent.RDS_MANAGEMENT: ["rds_discovery"],
    CloudIntent.RDS_SNAPSHOT_MANAGEMENT: ["rds_discovery"],
    CloudIntent.AURORA_CLUSTER_MANAGEMENT: ["rds_discovery"],
    CloudIntent.AURORA_SCALING: ["rds_discovery"],
    CloudIntent.AURORA_CLONING: ["rds_discovery"],
    CloudIntent.AURORA_GLOBAL_DATABASE: ["rds_discovery"],
    CloudIntent.ECS_DISCOVERY: [],
    CloudIntent.ECS_MANAGEMENT: ["ecs_discovery"],
    CloudIntent.EKS_DISCOVERY: [],
    CloudIntent.VPC_DISCOVERY: [],
    CloudIntent.ELB_DISCOVERY: [],
    CloudIntent.ROUTE53_DISCOVERY: [],
    CloudIntent.ROUTE53_MANAGEMENT: ["route53_discovery"],
    CloudIntent.AUTOSCALING_DISCOVERY: [],
    CloudIntent.AUTOSCALING_MANAGEMENT: ["autoscaling_discovery"],
    CloudIntent.CLOUDWATCH_METRICS: [],
    CloudIntent.SNS_DISCOVERY: [],
    CloudIntent.SNS_PUBLISH: ["sns_discovery"],
    CloudIntent.SQS_DISCOVERY: [],
    CloudIntent.SQS_MANAGEMENT: ["sqs_discovery"],
    CloudIntent.EVENTBRIDGE_DISCOVERY: [],
    CloudIntent.DYNAMODB_DISCOVERY: [],
    CloudIntent.DYNAMODB_READ: [],
    CloudIntent.DYNAMODB_WRITE: ["dynamodb_read"],
    CloudIntent.ELASTICACHE_DISCOVERY: [],
    CloudIntent.KMS_DISCOVERY: [],
    CloudIntent.KMS_CRYPTO: ["kms_discovery"],
    CloudIntent.IAM_DISCOVERY: [],
    CloudIntent.ACM_DISCOVERY: [],
    CloudIntent.APIGATEWAY_DISCOVERY: [],
    CloudIntent.STEPFUNCTIONS_DISCOVERY: [],
    CloudIntent.STEPFUNCTIONS_EXECUTION: ["stepfunctions_discovery"],
    CloudIntent.ATHENA_DISCOVERY: [],
    CloudIntent.ATHENA_QUERY_EXECUTION: ["athena_discovery"],
    CloudIntent.GLUE_DISCOVERY: [],
    CloudIntent.CLOUDFRONT_DISCOVERY: [],
    CloudIntent.CODEDEPLOY_DISCOVERY: [],
    CloudIntent.COST_EXPLORER: [],
}

# Mapping of intent to its human-readable question
CLOUD_INTENT_QUESTIONS = {
    CloudIntent.EC2_DISCOVERY: "Should I be able to see other EC2 instances in your account?",
    CloudIntent.EC2_MANAGEMENT: "Should I be able to start, stop, and manage EC2 instances?",
    CloudIntent.EC2_SNAPSHOT_MANAGEMENT: "Should I be able to create and manage EC2/EBS snapshots and AMIs?",
    CloudIntent.S3_READ: "Should I be able to read files from S3 buckets?",
    CloudIntent.S3_WRITE: "Should I be able to write files to S3 buckets?",
    CloudIntent.S3_BUCKET_DISCOVERY: "Should I be able to list and view S3 bucket configurations?",
    CloudIntent.S3_DELETE: "Should I be able to delete objects from S3 buckets?",
    CloudIntent.TERRAFORM_STATE: "Should I be able to manage Terraform state in S3 and DynamoDB?",
    CloudIntent.CLOUDFORMATION_DEPLOYMENT: "Should I be able to create and update CloudFormation stacks?",
    CloudIntent.CLOUDWATCH_LOGS: "Should I be able to view and write CloudWatch Logs?",
    CloudIntent.SECRETS_READ: "Should I be able to read secrets from Secrets Manager?",
    CloudIntent.LAMBDA_DISCOVERY: "Should I be able to see Lambda functions?",
    CloudIntent.LAMBDA_INVOKE: "Should I be able to invoke Lambda functions?",
    CloudIntent.RDS_DISCOVERY: "Should I be able to see RDS databases?",
    CloudIntent.RDS_MANAGEMENT: "Should I be able to start, stop, and reboot RDS databases?",
    CloudIntent.RDS_SNAPSHOT_MANAGEMENT: "Should I be able to create and manage RDS snapshots?",
    CloudIntent.AURORA_CLUSTER_MANAGEMENT: "Should I be able to manage Aurora clusters (failover, modify, add/remove instances)?",
    CloudIntent.AURORA_SCALING: "Should I be able to manage Aurora Serverless v2 scaling and capacity?",
    CloudIntent.AURORA_CLONING: "Should I be able to clone Aurora clusters?",
    CloudIntent.AURORA_GLOBAL_DATABASE: "Should I be able to manage Aurora Global Database operations?",
    CloudIntent.ECS_DISCOVERY: "Should I be able to see ECS clusters and services?",
    CloudIntent.ECS_MANAGEMENT: "Should I be able to update and manage ECS services?",
    CloudIntent.EKS_DISCOVERY: "Should I be able to see EKS clusters?",
    CloudIntent.VPC_DISCOVERY: "Should I be able to see VPCs, subnets, and security groups?",
    CloudIntent.ELB_DISCOVERY: "Should I be able to see load balancers?",
    CloudIntent.ROUTE53_DISCOVERY: "Should I be able to see Route 53 hosted zones and DNS records?",
    CloudIntent.ROUTE53_MANAGEMENT: "Should I be able to create and modify Route 53 DNS records?",
    CloudIntent.AUTOSCALING_DISCOVERY: "Should I be able to see Auto Scaling groups?",
    CloudIntent.AUTOSCALING_MANAGEMENT: "Should I be able to adjust Auto Scaling group capacity?",
    CloudIntent.CLOUDWATCH_METRICS: "Should I be able to read CloudWatch metrics?",
    CloudIntent.SNS_DISCOVERY: "Should I be able to see SNS topics?",
    CloudIntent.SNS_PUBLISH: "Should I be able to publish messages to SNS topics?",
    CloudIntent.SQS_DISCOVERY: "Should I be able to see SQS queues?",
    CloudIntent.SQS_MANAGEMENT: "Should I be able to send and manage messages in SQS queues?",
    CloudIntent.EVENTBRIDGE_DISCOVERY: "Should I be able to see EventBridge rules and event buses?",
    CloudIntent.DYNAMODB_DISCOVERY: "Should I be able to see DynamoDB tables?",
    CloudIntent.DYNAMODB_READ: "Should I be able to read items from DynamoDB tables?",
    CloudIntent.DYNAMODB_WRITE: "Should I be able to write items to DynamoDB tables?",
    CloudIntent.ELASTICACHE_DISCOVERY: "Should I be able to see ElastiCache clusters?",
    CloudIntent.KMS_DISCOVERY: "Should I be able to see KMS keys?",
    CloudIntent.KMS_CRYPTO: "Should I be able to encrypt and decrypt data using KMS keys?",
    CloudIntent.IAM_DISCOVERY: "Should I be able to see IAM roles and policies?",
    CloudIntent.ACM_DISCOVERY: "Should I be able to see ACM certificates?",
    CloudIntent.APIGATEWAY_DISCOVERY: "Should I be able to see API Gateway APIs?",
    CloudIntent.STEPFUNCTIONS_DISCOVERY: "Should I be able to see Step Functions state machines?",
    CloudIntent.STEPFUNCTIONS_EXECUTION: "Should I be able to start and stop Step Functions executions?",
    CloudIntent.ATHENA_DISCOVERY: "Should I be able to see Athena workgroups and queries?",
    CloudIntent.ATHENA_QUERY_EXECUTION: "Should I be able to execute Athena queries?",
    CloudIntent.GLUE_DISCOVERY: "Should I be able to see Glue databases and crawlers?",
    CloudIntent.CLOUDFRONT_DISCOVERY: "Should I be able to see CloudFront distributions?",
    CloudIntent.CODEDEPLOY_DISCOVERY: "Should I be able to see CodeDeploy applications and deployments?",
    CloudIntent.COST_EXPLORER: "Should I be able to read AWS cost and usage data?",
}

# Mapping of intent to its primary IAM verification action
CLOUD_INTENT_VERIFICATION_ACTIONS = {
    CloudIntent.EC2_DISCOVERY: "ec2:DescribeInstances",
    CloudIntent.EC2_MANAGEMENT: "ec2:StartInstances",
    CloudIntent.EC2_SNAPSHOT_MANAGEMENT: "ec2:CreateSnapshot",
    CloudIntent.S3_READ: "s3:GetObject",
    CloudIntent.S3_WRITE: "s3:PutObject",
    CloudIntent.S3_BUCKET_DISCOVERY: "s3:ListAllMyBuckets",
    CloudIntent.S3_DELETE: "s3:DeleteObject",
    CloudIntent.TERRAFORM_STATE: "s3:GetObject",
    CloudIntent.CLOUDFORMATION_DEPLOYMENT: "cloudformation:CreateStack",
    CloudIntent.CLOUDWATCH_LOGS: "logs:DescribeLogGroups",
    CloudIntent.SECRETS_READ: "secretsmanager:GetSecretValue",
    CloudIntent.LAMBDA_DISCOVERY: "lambda:ListFunctions",
    CloudIntent.LAMBDA_INVOKE: "lambda:InvokeFunction",
    CloudIntent.RDS_DISCOVERY: "rds:DescribeDBInstances",
    CloudIntent.RDS_MANAGEMENT: "rds:StartDBInstance",
    CloudIntent.RDS_SNAPSHOT_MANAGEMENT: "rds:CreateDBSnapshot",
    CloudIntent.AURORA_CLUSTER_MANAGEMENT: "rds:DescribeDBClusters",
    CloudIntent.AURORA_SCALING: "rds:DescribeDBClusterEndpoints",
    CloudIntent.AURORA_CLONING: "rds:RestoreDBClusterToPointInTime",
    CloudIntent.AURORA_GLOBAL_DATABASE: "rds:DescribeGlobalClusters",
    CloudIntent.ECS_DISCOVERY: "ecs:ListClusters",
    CloudIntent.ECS_MANAGEMENT: "ecs:UpdateService",
    CloudIntent.EKS_DISCOVERY: "eks:ListClusters",
    CloudIntent.VPC_DISCOVERY: "ec2:DescribeVpcs",
    CloudIntent.ELB_DISCOVERY: "elasticloadbalancing:DescribeLoadBalancers",
    CloudIntent.ROUTE53_DISCOVERY: "route53:ListHostedZones",
    CloudIntent.ROUTE53_MANAGEMENT: "route53:ChangeResourceRecordSets",
    CloudIntent.AUTOSCALING_DISCOVERY: "autoscaling:DescribeAutoScalingGroups",
    CloudIntent.AUTOSCALING_MANAGEMENT: "autoscaling:SetDesiredCapacity",
    CloudIntent.CLOUDWATCH_METRICS: "cloudwatch:GetMetricData",
    CloudIntent.SNS_DISCOVERY: "sns:ListTopics",
    CloudIntent.SNS_PUBLISH: "sns:Publish",
    CloudIntent.SQS_DISCOVERY: "sqs:ListQueues",
    CloudIntent.SQS_MANAGEMENT: "sqs:SendMessage",
    CloudIntent.EVENTBRIDGE_DISCOVERY: "events:ListRules",
    CloudIntent.DYNAMODB_DISCOVERY: "dynamodb:ListTables",
    CloudIntent.DYNAMODB_READ: "dynamodb:GetItem",
    CloudIntent.DYNAMODB_WRITE: "dynamodb:PutItem",
    CloudIntent.ELASTICACHE_DISCOVERY: "elasticache:DescribeCacheClusters",
    CloudIntent.KMS_DISCOVERY: "kms:ListKeys",
    CloudIntent.KMS_CRYPTO: "kms:Encrypt",
    CloudIntent.IAM_DISCOVERY: "iam:ListRoles",
    CloudIntent.ACM_DISCOVERY: "acm:ListCertificates",
    CloudIntent.APIGATEWAY_DISCOVERY: "apigateway:GET",
    CloudIntent.STEPFUNCTIONS_DISCOVERY: "states:ListStateMachines",
    CloudIntent.STEPFUNCTIONS_EXECUTION: "states:StartExecution",
    CloudIntent.ATHENA_DISCOVERY: "athena:ListWorkGroups",
    CloudIntent.ATHENA_QUERY_EXECUTION: "athena:StartQueryExecution",
    CloudIntent.GLUE_DISCOVERY: "glue:GetDatabases",
    CloudIntent.CLOUDFRONT_DISCOVERY: "cloudfront:ListDistributions",
    CloudIntent.CODEDEPLOY_DISCOVERY: "codedeploy:ListApplications",
    CloudIntent.COST_EXPLORER: "ce:GetCostAndUsage",
}
