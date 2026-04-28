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

from enum import Enum

class CloudIntent(str, Enum):
    # Generated from shared/constants/intents.json
    ACM_DISCOVERY = "acm_discovery"
    APIGATEWAY_DISCOVERY = "apigateway_discovery"
    ATHENA_DISCOVERY = "athena_discovery"
    ATHENA_QUERY_EXECUTION = "athena_query_execution"
    AURORA_CLONING = "aurora_cloning"
    AURORA_CLUSTER_MANAGEMENT = "aurora_cluster_management"
    AURORA_GLOBAL_DATABASE = "aurora_global_database"
    AURORA_SCALING = "aurora_scaling"
    AUTOSCALING_DISCOVERY = "autoscaling_discovery"
    AUTOSCALING_MANAGEMENT = "autoscaling_management"
    CLOUDFORMATION_DEPLOYMENT = "cloudformation_deployment"
    CLOUDFRONT_DISCOVERY = "cloudfront_discovery"
    CLOUDWATCH_LOGS = "cloudwatch_logs"
    CLOUDWATCH_METRICS = "cloudwatch_metrics"
    CODEDEPLOY_DISCOVERY = "codedeploy_discovery"
    COST_EXPLORER = "cost_explorer"
    DYNAMODB_DISCOVERY = "dynamodb_discovery"
    DYNAMODB_READ = "dynamodb_read"
    DYNAMODB_WRITE = "dynamodb_write"
    EC2_DISCOVERY = "ec2_discovery"
    EC2_MANAGEMENT = "ec2_management"
    EC2_SNAPSHOT_MANAGEMENT = "ec2_snapshot_management"
    ECS_DISCOVERY = "ecs_discovery"
    ECS_MANAGEMENT = "ecs_management"
    EKS_DISCOVERY = "eks_discovery"
    ELASTICACHE_DISCOVERY = "elasticache_discovery"
    ELB_DISCOVERY = "elb_discovery"
    EVENTBRIDGE_DISCOVERY = "eventbridge_discovery"
    GLUE_DISCOVERY = "glue_discovery"
    IAM_DISCOVERY = "iam_discovery"
    KMS_CRYPTO = "kms_crypto"
    KMS_DISCOVERY = "kms_discovery"
    LAMBDA_DISCOVERY = "lambda_discovery"
    LAMBDA_INVOKE = "lambda_invoke"
    RDS_DISCOVERY = "rds_discovery"
    RDS_MANAGEMENT = "rds_management"
    RDS_SNAPSHOT_MANAGEMENT = "rds_snapshot_management"
    ROUTE53_DISCOVERY = "route53_discovery"
    ROUTE53_MANAGEMENT = "route53_management"
    S3_BUCKET_DISCOVERY = "s3_bucket_discovery"
    S3_DELETE = "s3_delete"
    S3_READ = "s3_read"
    S3_WRITE = "s3_write"
    SECRETS_READ = "secrets_read"
    SNS_DISCOVERY = "sns_discovery"
    SNS_PUBLISH = "sns_publish"
    SQS_DISCOVERY = "sqs_discovery"
    SQS_MANAGEMENT = "sqs_management"
    STEPFUNCTIONS_DISCOVERY = "stepfunctions_discovery"
    STEPFUNCTIONS_EXECUTION = "stepfunctions_execution"
    TERRAFORM_STATE = "terraform_state"
    VPC_DISCOVERY = "vpc_discovery"

    # Legacy category aliases (keep for compatibility if needed by older code)
    FS_READ = "s3_read"
    FS_WRITE = "s3_write"
    NET_CONNECT = "ec2_management"
    NET_LISTEN = "elb_discovery"
    PROC_EXEC = "ec2_management"
    SYS_ADMIN = "iam_discovery"
    SECRET_READ = "secrets_read"

CLOUD_INTENT_QUESTIONS = {
    "acm_discovery": "Should I be able to see ACM certificates?",
    "apigateway_discovery": "Should I be able to see API Gateway APIs?",
    "athena_discovery": "Should I be able to see Athena workgroups and queries?",
    "athena_query_execution": "Should I be able to execute Athena queries?",
    "aurora_cloning": "Should I be able to clone Aurora clusters?",
    "aurora_cluster_management": "Should I be able to manage Aurora clusters (failover, modify, add/remove instances)?",
    "aurora_global_database": "Should I be able to manage Aurora Global Database operations?",
    "aurora_scaling": "Should I be able to manage Aurora Serverless v2 scaling and capacity?",
    "autoscaling_discovery": "Should I be able to see Auto Scaling groups?",
    "autoscaling_management": "Should I be able to adjust Auto Scaling group capacity?",
    "cloudformation_deployment": "Should I be able to create and update CloudFormation stacks?",
    "cloudfront_discovery": "Should I be able to see CloudFront distributions?",
    "cloudwatch_logs": "Should I be able to view and write CloudWatch Logs?",
    "cloudwatch_metrics": "Should I be able to read CloudWatch metrics?",
    "codedeploy_discovery": "Should I be able to see CodeDeploy applications and deployments?",
    "cost_explorer": "Should I be able to read AWS cost and usage data?",
    "dynamodb_discovery": "Should I be able to see DynamoDB tables?",
    "dynamodb_read": "Should I be able to read items from DynamoDB tables?",
    "dynamodb_write": "Should I be able to write items to DynamoDB tables?",
    "ec2_discovery": "Should I be able to see other EC2 instances in your account?",
    "ec2_management": "Should I be able to start, stop, and manage EC2 instances?",
    "ec2_snapshot_management": "Should I be able to create and manage EC2/EBS snapshots and AMIs?",
    "ecs_discovery": "Should I be able to see ECS clusters and services?",
    "ecs_management": "Should I be able to update and manage ECS services?",
    "eks_discovery": "Should I be able to see EKS clusters?",
    "elasticache_discovery": "Should I be able to see ElastiCache clusters?",
    "elb_discovery": "Should I be able to see load balancers?",
    "eventbridge_discovery": "Should I be able to see EventBridge rules and event buses?",
    "glue_discovery": "Should I be able to see Glue databases and crawlers?",
    "iam_discovery": "Should I be able to see IAM roles and policies?",
    "kms_crypto": "Should I be able to encrypt and decrypt data using KMS keys?",
    "kms_discovery": "Should I be able to see KMS keys?",
    "lambda_discovery": "Should I be able to see Lambda functions?",
    "lambda_invoke": "Should I be able to invoke Lambda functions?",
    "rds_discovery": "Should I be able to see RDS databases?",
    "rds_management": "Should I be able to start, stop, and reboot RDS databases?",
    "rds_snapshot_management": "Should I be able to create and manage RDS snapshots?",
    "route53_discovery": "Should I be able to see Route 53 hosted zones and DNS records?",
    "route53_management": "Should I be able to create and modify Route 53 DNS records?",
    "s3_bucket_discovery": "Should I be able to list and view S3 bucket configurations?",
    "s3_delete": "Should I be able to delete objects from S3 buckets?",
    "s3_read": "Should I be able to read files from S3 buckets?",
    "s3_write": "Should I be able to write files to S3 buckets?",
    "secrets_read": "Should I be able to read secrets from Secrets Manager?",
    "sns_discovery": "Should I be able to see SNS topics?",
    "sns_publish": "Should I be able to publish messages to SNS topics?",
    "sqs_discovery": "Should I be able to see SQS queues?",
    "sqs_management": "Should I be able to send and manage messages in SQS queues?",
    "stepfunctions_discovery": "Should I be able to see Step Functions state machines?",
    "stepfunctions_execution": "Should I be able to start and stop Step Functions executions?",
    "terraform_state": "Should I be able to manage Terraform state in S3 and DynamoDB?",
    "vpc_discovery": "Should I be able to see VPCs, subnets, and security groups?"
}

CLOUD_INTENT_VERIFICATION_ACTIONS = {
    "acm_discovery": "acm:ListCertificates",
    "apigateway_discovery": "apigateway:GET",
    "athena_discovery": "athena:ListWorkGroups",
    "athena_query_execution": "athena:StartQueryExecution",
    "aurora_cloning": "rds:RestoreDBClusterToPointInTime",
    "aurora_cluster_management": "rds:DescribeDBClusters",
    "aurora_global_database": "rds:DescribeGlobalClusters",
    "aurora_scaling": "rds:DescribeDBClusterEndpoints",
    "autoscaling_discovery": "autoscaling:DescribeAutoScalingGroups",
    "autoscaling_management": "autoscaling:SetDesiredCapacity",
    "cloudformation_deployment": "cloudformation:CreateStack",
    "cloudfront_discovery": "cloudfront:ListDistributions",
    "cloudwatch_logs": "logs:DescribeLogGroups",
    "cloudwatch_metrics": "cloudwatch:GetMetricData",
    "codedeploy_discovery": "codedeploy:ListApplications",
    "cost_explorer": "ce:GetCostAndUsage",
    "dynamodb_discovery": "dynamodb:ListTables",
    "dynamodb_read": "dynamodb:GetItem",
    "dynamodb_write": "dynamodb:PutItem",
    "ec2_discovery": "ec2:DescribeInstances",
    "ec2_management": "ec2:StartInstances",
    "ec2_snapshot_management": "ec2:CreateSnapshot",
    "ecs_discovery": "ecs:ListClusters",
    "ecs_management": "ecs:UpdateService",
    "eks_discovery": "eks:ListClusters",
    "elasticache_discovery": "elasticache:DescribeCacheClusters",
    "elb_discovery": "elasticloadbalancing:DescribeLoadBalancers",
    "eventbridge_discovery": "events:ListRules",
    "glue_discovery": "glue:GetDatabases",
    "iam_discovery": "iam:ListRoles",
    "kms_crypto": "kms:Encrypt",
    "kms_discovery": "kms:ListKeys",
    "lambda_discovery": "lambda:ListFunctions",
    "lambda_invoke": "lambda:InvokeFunction",
    "rds_discovery": "rds:DescribeDBInstances",
    "rds_management": "rds:StartDBInstance",
    "rds_snapshot_management": "rds:CreateDBSnapshot",
    "route53_discovery": "route53:ListHostedZones",
    "route53_management": "route53:ChangeResourceRecordSets",
    "s3_bucket_discovery": "s3:ListAllMyBuckets",
    "s3_delete": "s3:DeleteObject",
    "s3_read": "s3:GetObject",
    "s3_write": "s3:PutObject",
    "secrets_read": "secretsmanager:GetSecretValue",
    "sns_discovery": "sns:ListTopics",
    "sns_publish": "sns:Publish",
    "sqs_discovery": "sqs:ListQueues",
    "sqs_management": "sqs:SendMessage",
    "stepfunctions_discovery": "states:ListStateMachines",
    "stepfunctions_execution": "states:StartExecution",
    "terraform_state": "s3:GetObject",
    "vpc_discovery": "ec2:DescribeVpcs"
}

CLOUD_INTENT_DEPENDENCIES = {
    "athena_query_execution": ["athena_discovery"],
    "aurora_cloning": ["rds_discovery"],
    "aurora_cluster_management": ["rds_discovery"],
    "aurora_global_database": ["rds_discovery"],
    "aurora_scaling": ["rds_discovery"],
    "autoscaling_management": ["autoscaling_discovery"],
    "dynamodb_write": ["dynamodb_read"],
    "ec2_management": ["ec2_discovery"],
    "ec2_snapshot_management": ["ec2_discovery"],
    "ecs_management": ["ecs_discovery"],
    "kms_crypto": ["kms_discovery"],
    "lambda_invoke": ["lambda_discovery"],
    "rds_management": ["rds_discovery"],
    "rds_snapshot_management": ["rds_discovery"],
    "route53_management": ["route53_discovery"],
    "s3_delete": ["s3_read"],
    "s3_write": ["s3_read"],
    "sns_publish": ["sns_discovery"],
    "sqs_management": ["sqs_discovery"],
    "stepfunctions_execution": ["stepfunctions_discovery"]
}
