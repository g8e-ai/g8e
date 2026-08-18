// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package constants

// CloudIntent is a typed string for authoritative IAM policy intents.
type CloudIntent string

const (
	IntentEc2Discovery             CloudIntent = "ec2_discovery"
	IntentEc2Management            CloudIntent = "ec2_management"
	IntentEc2SnapshotManagement    CloudIntent = "ec2_snapshot_management"
	IntentS3Read                   CloudIntent = "s3_read"
	IntentS3Write                  CloudIntent = "s3_write"
	IntentS3BucketDiscovery        CloudIntent = "s3_bucket_discovery"
	IntentS3Delete                 CloudIntent = "s3_delete"
	IntentTerraformState           CloudIntent = "terraform_state"
	IntentCloudformationDeployment CloudIntent = "cloudformation_deployment"
	IntentCloudwatchLogs           CloudIntent = "cloudwatch_logs"
	IntentSecretsRead              CloudIntent = "secrets_read"
	IntentLambdaDiscovery          CloudIntent = "lambda_discovery"
	IntentLambdaInvoke             CloudIntent = "lambda_invoke"
	IntentRdsDiscovery             CloudIntent = "rds_discovery"
	IntentRdsManagement            CloudIntent = "rds_management"
	IntentRdsSnapshotManagement    CloudIntent = "rds_snapshot_management"
	IntentAuroraClusterManagement  CloudIntent = "aurora_cluster_management"
	IntentAuroraScaling            CloudIntent = "aurora_scaling"
	IntentAuroraCloning            CloudIntent = "aurora_cloning"
	IntentAuroraGlobalDatabase     CloudIntent = "aurora_global_database"
	IntentEcsDiscovery             CloudIntent = "ecs_discovery"
	IntentEcsManagement            CloudIntent = "ecs_management"
	IntentEksDiscovery             CloudIntent = "eks_discovery"
	IntentVpcDiscovery             CloudIntent = "vpc_discovery"
	IntentElbDiscovery             CloudIntent = "elb_discovery"
	IntentRoute53Discovery         CloudIntent = "route53_discovery"
	IntentRoute53Management        CloudIntent = "route53_management"
	IntentAutoscalingDiscovery     CloudIntent = "autoscaling_discovery"
	IntentAutoscalingManagement    CloudIntent = "autoscaling_management"
	IntentCloudwatchMetrics        CloudIntent = "cloudwatch_metrics"
	IntentSnsDiscovery             CloudIntent = "sns_discovery"
	IntentSnsPublish               CloudIntent = "sns_publish"
	IntentSqsDiscovery             CloudIntent = "sqs_discovery"
	IntentSqsManagement            CloudIntent = "sqs_management"
	IntentEventbridgeDiscovery     CloudIntent = "eventbridge_discovery"
	IntentDynamodbDiscovery        CloudIntent = "dynamodb_discovery"
	IntentDynamodbRead             CloudIntent = "dynamodb_read"
	IntentDynamodbWrite            CloudIntent = "dynamodb_write"
	IntentElasticacheDiscovery     CloudIntent = "elasticache_discovery"
	IntentKmsDiscovery             CloudIntent = "kms_discovery"
	IntentKmsCrypto                CloudIntent = "kms_crypto"
	IntentIamDiscovery             CloudIntent = "iam_discovery"
	IntentAcmDiscovery             CloudIntent = "acm_discovery"
	IntentApigatewayDiscovery      CloudIntent = "apigateway_discovery"
	IntentStepfunctionsDiscovery   CloudIntent = "stepfunctions_discovery"
	IntentStepfunctionsExecution   CloudIntent = "stepfunctions_execution"
	IntentAthenaDiscovery          CloudIntent = "athena_discovery"
	IntentAthenaQueryExecution     CloudIntent = "athena_query_execution"
	IntentGlueDiscovery            CloudIntent = "glue_discovery"
	IntentCloudfrontDiscovery      CloudIntent = "cloudfront_discovery"
	IntentCodedeployDiscovery      CloudIntent = "codedeploy_discovery"
	IntentCostExplorer             CloudIntent = "cost_explorer"
)
