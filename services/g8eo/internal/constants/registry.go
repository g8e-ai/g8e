// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package constants

// Entry represents a constant with its value and naming metadata.
type Entry struct {
	Value       string `json:"value"`
	GoConst     string `json:"_go_const"`
	PythonConst string `json:"_python_const"`
}

// KVKeysSnapshot represents the nested structure for KV keys.
type KVKeysSnapshot struct {
	CachePrefix string            `json:"cache.prefix"`
	KeySchema   map[string]string `json:"key.schema"`
	SessionType map[string]string `json:"session.type"`
}

// PathsSnapshot represents the nested structure for paths.
type PathsSnapshot struct {
	Infra map[string]string `json:"infra"`
	G8ee  map[string]string `json:"g8ee"`
	Ports map[string]int    `json:"ports"`
	Host  string            `json:"host"`
}

// DocumentIdsSnapshot represents the nested structure for document IDs.
type DocumentIdsSnapshot struct {
	DocumentIds map[string]Entry `json:"document_ids"`
}

// StatusSnapshot represents the nested structure for status values.
type StatusSnapshot struct {
	AttachmentType    map[string]Entry `json:"attachment.type"`
	UserRole          map[string]Entry `json:"user_role"`
	UserStatus        map[string]Entry `json:"user_status"`
	OperatorStatus    map[string]Entry `json:"operator_status"`
	ExecutionStatus   map[string]Entry `json:"execution_status"`
	TribunalOutcome   map[string]Entry `json:"tribunal.outcome"`
	ApprovalErrorType map[string]Entry `json:"approval.error.type"`
	LlmModels         map[string]Entry `json:"llm.models"`
}

// Snapshot is the complete constants registry snapshot.
type Snapshot struct {
	Collections map[string]Entry    `json:"collections"`
	Events      map[string]Entry    `json:"events"`
	Status      StatusSnapshot      `json:"status"`
	Senders     map[string]Entry    `json:"senders"`
	KVKeys      KVKeysSnapshot      `json:"kv_keys"`
	Channels    map[string]Entry    `json:"channels"`
	PubSub      map[string]Entry    `json:"pubsub"`
	Intents     map[string]Entry    `json:"intents"`
	Prompts     map[string]Entry    `json:"prompts"`
	Headers     map[string]Entry    `json:"headers"`
	DocumentIds DocumentIdsSnapshot `json:"document_ids"`
	Platform    map[string]Entry    `json:"platform"`
	Agents      map[string]string   `json:"agents"`
	Paths       PathsSnapshot       `json:"paths"`
	Ports       map[string]int      `json:"ports"`
	EnvVars     map[string]string   `json:"env_vars"`
	Timestamp   map[string]string   `json:"timestamp"`
	ApiPaths    interface{}         `json:"api_paths"`
}

// Registry returns the complete constants snapshot.
func Registry() Snapshot {
	return Snapshot{
		Collections: map[string]Entry{
			"users":                   {Value: string(CollectionUsers), GoConst: "CollectionUsers", PythonConst: "USERS"},
			"web_sessions":            {Value: string(CollectionWebSessions), GoConst: "CollectionWebSessions", PythonConst: "WEB_SESSIONS"},
			"operator_sessions":       {Value: string(CollectionOperatorSessions), GoConst: "CollectionOperatorSessions", PythonConst: "OPERATOR_SESSIONS"},
			"cli_sessions":            {Value: string(CollectionCLISessions), GoConst: "CollectionCLISessions", PythonConst: "CLI_SESSIONS"},
			"login_audit":             {Value: string(CollectionLoginAudit), GoConst: "CollectionLoginAudit", PythonConst: "LOGIN_AUDIT"},
			"auth_admin_audit":        {Value: string(CollectionAuthAdminAudit), GoConst: "CollectionAuthAdminAudit", PythonConst: "AUTH_ADMIN_AUDIT"},
			"account_locks":           {Value: string(CollectionAccountLocks), GoConst: "CollectionAccountLocks", PythonConst: "ACCOUNT_LOCKS"},
			"api_keys":                {Value: string(CollectionAPIKeys), GoConst: "CollectionAPIKeys", PythonConst: "API_KEYS"},
			"organizations":           {Value: string(CollectionOrganizations), GoConst: "CollectionOrganizations", PythonConst: "ORGANIZATIONS"},
			"operators":               {Value: string(CollectionOperators), GoConst: "CollectionOperators", PythonConst: "OPERATORS"},
			"operator_usage":          {Value: string(CollectionOperatorUsage), GoConst: "CollectionOperatorUsage", PythonConst: "OPERATOR_USAGE"},
			"cases":                   {Value: string(CollectionCases), GoConst: "CollectionCases", PythonConst: "CASES"},
			"investigations":          {Value: string(CollectionInvestigations), GoConst: "CollectionInvestigations", PythonConst: "INVESTIGATIONS"},
			"tasks":                   {Value: string(CollectionTasks), GoConst: "CollectionTasks", PythonConst: "TASKS"},
			"memories":                {Value: string(CollectionMemories), GoConst: "CollectionMemories", PythonConst: "MEMORIES"},
			"settings":                {Value: string(CollectionSettings), GoConst: "CollectionSettings", PythonConst: "SETTINGS"},
			"console_audit":           {Value: string(CollectionConsoleAudit), GoConst: "CollectionConsoleAudit", PythonConst: "CONSOLE_AUDIT"},
			"bound_sessions":          {Value: string(CollectionBoundSessions), GoConst: "CollectionBoundSessions", PythonConst: "BOUND_SESSIONS"},
			"passkey_challenges":      {Value: string(CollectionPasskeyChallenges), GoConst: "CollectionPasskeyChallenges", PythonConst: "PASSKEY_CHALLENGES"},
			"tribunal_commands":       {Value: string(CollectionTribunalCommands), GoConst: "CollectionTribunalCommands", PythonConst: "TRIBUNAL_COMMANDS"},
			"agent_activity_metadata": {Value: string(CollectionAgentActivityMetadata), GoConst: "CollectionAgentActivityMetadata", PythonConst: "AGENT_ACTIVITY_METADATA"},
			"reputation_state":        {Value: string(CollectionReputationState), GoConst: "CollectionReputationState", PythonConst: "REPUTATION_STATE"},
			"reputation_commitments":  {Value: string(CollectionReputationCommitments), GoConst: "CollectionReputationCommitments", PythonConst: "REPUTATION_COMMITMENTS"},
			"stake_resolutions":       {Value: string(CollectionStakeResolutions), GoConst: "CollectionStakeResolutions", PythonConst: "STAKE_RESOLUTIONS"},
			"revoked_certificates":    {Value: string(CollectionRevokedCertificates), GoConst: "CollectionRevokedCertificates", PythonConst: "REVOKED_CERTIFICATES"},
			"trusted_signers":         {Value: string(CollectionTrustedSigners), GoConst: "CollectionTrustedSigners", PythonConst: "TRUSTED_SIGNERS"},
			"chaos_events":            {Value: string(CollectionChaosEvents), GoConst: "CollectionChaosEvents", PythonConst: "CHAOS_EVENTS"},
		},
		Channels: map[string]Entry{
			"Subscribe":   {Value: PubSubActionSubscribe, GoConst: "PubSubActionSubscribe", PythonConst: "SUBSCRIBE"},
			"PSubscribe":  {Value: PubSubActionPSubscribe, GoConst: "PubSubActionPSubscribe", PythonConst: "P_SUBSCRIBE"},
			"Unsubscribe": {Value: PubSubActionUnsubscribe, GoConst: "PubSubActionUnsubscribe", PythonConst: "UNSUBSCRIBE"},
			"Publish":     {Value: PubSubActionPublish, GoConst: "PubSubActionPublish", PythonConst: "PUBLISH"},
			"Message":     {Value: PubSubEventMessage, GoConst: "PubSubEventMessage", PythonConst: "MESSAGE"},
			"PMessage":    {Value: PubSubEventPMessage, GoConst: "PubSubEventPMessage", PythonConst: "P_MESSAGE"},
			"Subscribed":  {Value: PubSubEventSubscribed, GoConst: "PubSubEventSubscribed", PythonConst: "SUBSCRIBED"},
		},
		DocumentIds: DocumentIdsSnapshot{
			DocumentIds: map[string]Entry{
				"app_settings":         {Value: string(DocIDAppSettings), GoConst: "DocIDAppSettings", PythonConst: "PLATFORM_SETTINGS"},
				"user_settings_prefix": {Value: string(DocIDUserSettingsPrefix), GoConst: "DocIDUserSettingsPrefix", PythonConst: "USER_SETTINGS_PREFIX"},
			},
		},
		Headers: map[string]Entry{
			"DeviceToken":                   {Value: HeaderDeviceToken, GoConst: "HeaderDeviceToken", PythonConst: "DEVICE_TOKEN"},
			"Accept":                        {Value: HeaderAccept, GoConst: "HeaderAccept", PythonConst: "ACCEPT"},
			"AcceptLanguage":                {Value: HeaderAcceptLanguage, GoConst: "HeaderAcceptLanguage", PythonConst: "ACCEPT_LANGUAGE"},
			"ContentLanguage":               {Value: HeaderContentLanguage, GoConst: "HeaderContentLanguage", PythonConst: "CONTENT_LANGUAGE"},
			"Authorization":                 {Value: HeaderAuthorization, GoConst: "HeaderAuthorization", PythonConst: "AUTHORIZATION"},
			"APIKey":                        {Value: HeaderAPIKey, GoConst: "HeaderAPIKey", PythonConst: "API_KEY"},
			"RequestedWith":                 {Value: HeaderRequestedWith, GoConst: "HeaderRequestedWith", PythonConst: "REQUESTED_WITH"},
			"UserAgent":                     {Value: HeaderUserAgent, GoConst: "HeaderUserAgent", PythonConst: "USER_AGENT"},
			"CacheControl":                  {Value: HeaderCacheControl, GoConst: "HeaderCacheControl", PythonConst: "CACHE_CONTROL"},
			"Pragma":                        {Value: HeaderPragma, GoConst: "HeaderPragma", PythonConst: "PRAGMA"},
			"Cookie":                        {Value: HeaderCookie, GoConst: "HeaderCookie", PythonConst: "COOKIE"},
			"SetCookie":                     {Value: HeaderSetCookie, GoConst: "HeaderSetCookie", PythonConst: "SET_COOKIE"},
			"LastEventID":                   {Value: HeaderLastEventID, GoConst: "HeaderLastEventID", PythonConst: "LAST_EVENT_ID"},
			"ContentType":                   {Value: HeaderContentType, GoConst: "HeaderContentType", PythonConst: "CONTENT_TYPE"},
			"ContentDisposition":            {Value: HeaderContentDisposition, GoConst: "HeaderContentDisposition", PythonConst: "CONTENT_DISPOSITION"},
			"ContentLength":                 {Value: HeaderContentLength, GoConst: "HeaderContentLength", PythonConst: "CONTENT_LENGTH"},
			"AccessControlRequestHeaders":   {Value: HeaderAccessControlRequestHeaders, GoConst: "HeaderAccessControlRequestHeaders", PythonConst: "ACCESS_CONTROL_REQUEST_HEADERS"},
			"AccessControlRequestMethod":    {Value: HeaderAccessControlRequestMethod, GoConst: "HeaderAccessControlRequestMethod", PythonConst: "ACCESS_CONTROL_REQUEST_METHOD"},
			"AccessControlAllowOrigin":      {Value: HeaderAccessControlAllowOrigin, GoConst: "HeaderAccessControlAllowOrigin", PythonConst: "ACCESS_CONTROL_ALLOW_ORIGIN"},
			"AccessControlAllowCredentials": {Value: HeaderAccessControlAllowCredentials, GoConst: "HeaderAccessControlAllowCredentials", PythonConst: "ACCESS_CONTROL_ALLOW_CREDENTIALS"},
			"XAccelBuffering":               {Value: HeaderXAccelBuffering, GoConst: "HeaderXAccelBuffering", PythonConst: "X_ACCEL_BUFFERING"},
			"XForwardedProto":               {Value: HeaderXForwardedProto, GoConst: "HeaderXForwardedProto", PythonConst: "X_FORWARDED_PROTO"},
			"XForwardedHost":                {Value: HeaderXForwardedHost, GoConst: "HeaderXForwardedHost", PythonConst: "X_FORWARDED_HOST"},
			"XForwardedFor":                 {Value: HeaderXForwardedFor, GoConst: "HeaderXForwardedFor", PythonConst: "X_FORWARDED_FOR"},
			"CLISessionID":                  {Value: HeaderCLISessionID, GoConst: "HeaderCLISessionID", PythonConst: "CLI_SESSION_ID"},
			"XRequestTimestamp":             {Value: HeaderXRequestTimestamp, GoConst: "HeaderXRequestTimestamp", PythonConst: "X_REQUEST_TIMESTAMP"},
			"XProxyUserID":                  {Value: HeaderXProxyUserID, GoConst: "HeaderXProxyUserID", PythonConst: "X_PROXY_USER_ID"},
			"XProxyUserEmail":               {Value: HeaderXProxyUserEmail, GoConst: "HeaderXProxyUserEmail", PythonConst: "X_PROXY_USER_EMAIL"},
			"XProxyOrganizationID":          {Value: HeaderXProxyOrganizationID, GoConst: "HeaderXProxyOrganizationID", PythonConst: "X_PROXY_ORGANIZATION_ID"},
			"WebSessionID":                  {Value: HeaderWebSessionID, GoConst: "HeaderWebSessionID", PythonConst: "WEB_SESSION_ID"},
			"OperatorSessionID":             {Value: HeaderOperatorSessionID, GoConst: "HeaderOperatorSessionID", PythonConst: "OPERATOR_SESSION_ID"},
			"OperatorID":                    {Value: HeaderOperatorID, GoConst: "HeaderOperatorID", PythonConst: "OPERATOR_ID"},
			"OperatorAPIKey":                {Value: HeaderOperatorAPIKey, GoConst: "HeaderOperatorAPIKey", PythonConst: "OPERATOR_API_KEY"},
			"SystemFingerprint":             {Value: HeaderSystemFingerprint, GoConst: "HeaderSystemFingerprint", PythonConst: "SYSTEM_FINGERPRINT"},
			"RequestID":                     {Value: HeaderRequestID, GoConst: "HeaderRequestID", PythonConst: "REQUEST_ID"},
			"UserID":                        {Value: HeaderUserID, GoConst: "HeaderUserID", PythonConst: "USER_ID"},
			"CaseID":                        {Value: HeaderCaseID, GoConst: "HeaderCaseID", PythonConst: "CASE_ID"},
			"OrganizationID":                {Value: HeaderOrganizationID, GoConst: "HeaderOrganizationID", PythonConst: "ORGANIZATION_ID"},
			"InvestigationID":               {Value: HeaderInvestigationID, GoConst: "HeaderInvestigationID", PythonConst: "INVESTIGATION_ID"},
			"TaskID":                        {Value: HeaderTaskID, GoConst: "HeaderTaskID", PythonConst: "TASK_ID"},
			"BoundOperators":                {Value: HeaderBoundOperators, GoConst: "HeaderBoundOperators", PythonConst: "BOUND_OPERATORS"},
			"SourceComponent":               {Value: HeaderSourceComponent, GoConst: "HeaderSourceComponent", PythonConst: "SOURCE_COMPONENT"},
		},
		Intents: map[string]Entry{
			"Ec2Discovery":             {Value: string(IntentEc2Discovery), GoConst: "IntentEc2Discovery", PythonConst: "EC2_DISCOVERY"},
			"Ec2Management":            {Value: string(IntentEc2Management), GoConst: "IntentEc2Management", PythonConst: "EC2_MANAGEMENT"},
			"Ec2SnapshotManagement":    {Value: string(IntentEc2SnapshotManagement), GoConst: "IntentEc2SnapshotManagement", PythonConst: "EC2_SNAPSHOT_MANAGEMENT"},
			"S3Read":                   {Value: string(IntentS3Read), GoConst: "IntentS3Read", PythonConst: "S3_READ"},
			"S3Write":                  {Value: string(IntentS3Write), GoConst: "IntentS3Write", PythonConst: "S3_WRITE"},
			"S3BucketDiscovery":        {Value: string(IntentS3BucketDiscovery), GoConst: "IntentS3BucketDiscovery", PythonConst: "S3_BUCKET_DISCOVERY"},
			"S3Delete":                 {Value: string(IntentS3Delete), GoConst: "IntentS3Delete", PythonConst: "S3_DELETE"},
			"TerraformState":           {Value: string(IntentTerraformState), GoConst: "IntentTerraformState", PythonConst: "TERRAFORM_STATE"},
			"CloudformationDeployment": {Value: string(IntentCloudformationDeployment), GoConst: "IntentCloudformationDeployment", PythonConst: "CLOUDFORMATION_DEPLOYMENT"},
			"CloudwatchLogs":           {Value: string(IntentCloudwatchLogs), GoConst: "IntentCloudwatchLogs", PythonConst: "CLOUDWATCH_LOGS"},
			"SecretsRead":              {Value: string(IntentSecretsRead), GoConst: "IntentSecretsRead", PythonConst: "SECRETS_READ"},
			"LambdaDiscovery":          {Value: string(IntentLambdaDiscovery), GoConst: "IntentLambdaDiscovery", PythonConst: "LAMBDA_DISCOVERY"},
			"LambdaInvoke":             {Value: string(IntentLambdaInvoke), GoConst: "IntentLambdaInvoke", PythonConst: "LAMBDA_INVOKE"},
			"RdsDiscovery":             {Value: string(IntentRdsDiscovery), GoConst: "IntentRdsDiscovery", PythonConst: "RDS_DISCOVERY"},
			"RdsManagement":            {Value: string(IntentRdsManagement), GoConst: "IntentRdsManagement", PythonConst: "RDS_MANAGEMENT"},
			"RdsSnapshotManagement":    {Value: string(IntentRdsSnapshotManagement), GoConst: "IntentRdsSnapshotManagement", PythonConst: "RDS_SNAPSHOT_MANAGEMENT"},
			"AuroraClusterManagement":  {Value: string(IntentAuroraClusterManagement), GoConst: "IntentAuroraClusterManagement", PythonConst: "AURORA_CLUSTER_MANAGEMENT"},
			"AuroraScaling":            {Value: string(IntentAuroraScaling), GoConst: "IntentAuroraScaling", PythonConst: "AURORA_SCALING"},
			"AuroraCloning":            {Value: string(IntentAuroraCloning), GoConst: "IntentAuroraCloning", PythonConst: "AURORA_CLONING"},
			"AuroraGlobalDatabase":     {Value: string(IntentAuroraGlobalDatabase), GoConst: "IntentAuroraGlobalDatabase", PythonConst: "AURORA_GLOBAL_DATABASE"},
			"EcsDiscovery":             {Value: string(IntentEcsDiscovery), GoConst: "IntentEcsDiscovery", PythonConst: "ECS_DISCOVERY"},
			"EcsManagement":            {Value: string(IntentEcsManagement), GoConst: "IntentEcsManagement", PythonConst: "ECS_MANAGEMENT"},
			"EksDiscovery":             {Value: string(IntentEksDiscovery), GoConst: "IntentEksDiscovery", PythonConst: "EKS_DISCOVERY"},
			"VpcDiscovery":             {Value: string(IntentVpcDiscovery), GoConst: "IntentVpcDiscovery", PythonConst: "VPC_DISCOVERY"},
			"ElbDiscovery":             {Value: string(IntentElbDiscovery), GoConst: "IntentElbDiscovery", PythonConst: "ELB_DISCOVERY"},
			"Route53Discovery":         {Value: string(IntentRoute53Discovery), GoConst: "IntentRoute53Discovery", PythonConst: "ROUTE53_DISCOVERY"},
			"Route53Management":        {Value: string(IntentRoute53Management), GoConst: "IntentRoute53Management", PythonConst: "ROUTE53_MANAGEMENT"},
			"AutoscalingDiscovery":     {Value: string(IntentAutoscalingDiscovery), GoConst: "IntentAutoscalingDiscovery", PythonConst: "AUTOSCALING_DISCOVERY"},
			"AutoscalingManagement":    {Value: string(IntentAutoscalingManagement), GoConst: "IntentAutoscalingManagement", PythonConst: "AUTOSCALING_MANAGEMENT"},
			"CloudwatchMetrics":        {Value: string(IntentCloudwatchMetrics), GoConst: "IntentCloudwatchMetrics", PythonConst: "CLOUDWATCH_METRICS"},
			"SnsDiscovery":             {Value: string(IntentSnsDiscovery), GoConst: "IntentSnsDiscovery", PythonConst: "SNS_DISCOVERY"},
			"SnsPublish":               {Value: string(IntentSnsPublish), GoConst: "IntentSnsPublish", PythonConst: "SNS_PUBLISH"},
			"SqsDiscovery":             {Value: string(IntentSqsDiscovery), GoConst: "IntentSqsDiscovery", PythonConst: "SQS_DISCOVERY"},
			"SqsManagement":            {Value: string(IntentSqsManagement), GoConst: "IntentSqsManagement", PythonConst: "SQS_MANAGEMENT"},
			"EventbridgeDiscovery":     {Value: string(IntentEventbridgeDiscovery), GoConst: "IntentEventbridgeDiscovery", PythonConst: "EVENTBRIDGE_DISCOVERY"},
			"DynamodbDiscovery":        {Value: string(IntentDynamodbDiscovery), GoConst: "IntentDynamodbDiscovery", PythonConst: "DYNAMODB_DISCOVERY"},
			"DynamodbRead":             {Value: string(IntentDynamodbRead), GoConst: "IntentDynamodbRead", PythonConst: "DYNAMODB_READ"},
			"DynamodbWrite":            {Value: string(IntentDynamodbWrite), GoConst: "IntentDynamodbWrite", PythonConst: "DYNAMODB_WRITE"},
			"ElasticacheDiscovery":     {Value: string(IntentElasticacheDiscovery), GoConst: "IntentElasticacheDiscovery", PythonConst: "ELASTICACHE_DISCOVERY"},
			"KmsDiscovery":             {Value: string(IntentKmsDiscovery), GoConst: "IntentKmsDiscovery", PythonConst: "KMS_DISCOVERY"},
			"KmsCrypto":                {Value: string(IntentKmsCrypto), GoConst: "IntentKmsCrypto", PythonConst: "KMS_CRYPTO"},
			"IamDiscovery":             {Value: string(IntentIamDiscovery), GoConst: "IntentIamDiscovery", PythonConst: "IAM_DISCOVERY"},
			"AcmDiscovery":             {Value: string(IntentAcmDiscovery), GoConst: "IntentAcmDiscovery", PythonConst: "ACM_DISCOVERY"},
			"ApigatewayDiscovery":      {Value: string(IntentApigatewayDiscovery), GoConst: "IntentApigatewayDiscovery", PythonConst: "APIGATEWAY_DISCOVERY"},
			"StepfunctionsDiscovery":   {Value: string(IntentStepfunctionsDiscovery), GoConst: "IntentStepfunctionsDiscovery", PythonConst: "STEPFUNCTIONS_DISCOVERY"},
			"StepfunctionsExecution":   {Value: string(IntentStepfunctionsExecution), GoConst: "IntentStepfunctionsExecution", PythonConst: "STEPFUNCTIONS_EXECUTION"},
			"AthenaDiscovery":          {Value: string(IntentAthenaDiscovery), GoConst: "IntentAthenaDiscovery", PythonConst: "ATHENA_DISCOVERY"},
			"AthenaQueryExecution":     {Value: string(IntentAthenaQueryExecution), GoConst: "IntentAthenaQueryExecution", PythonConst: "ATHENA_QUERY_EXECUTION"},
			"GlueDiscovery":            {Value: string(IntentGlueDiscovery), GoConst: "IntentGlueDiscovery", PythonConst: "GLUE_DISCOVERY"},
			"CloudfrontDiscovery":      {Value: string(IntentCloudfrontDiscovery), GoConst: "IntentCloudfrontDiscovery", PythonConst: "CLOUDFRONT_DISCOVERY"},
			"CodedeployDiscovery":      {Value: string(IntentCodedeployDiscovery), GoConst: "IntentCodedeployDiscovery", PythonConst: "CODEPLOY_DISCOVERY"},
			"CostExplorer":             {Value: string(IntentCostExplorer), GoConst: "IntentCostExplorer", PythonConst: "COST_EXPLORER"},
		},
		KVKeys: KVKeysSnapshot{
			CachePrefix: KVCachePrefix,
			KeySchema: map[string]string{
				"CacheDoc":        KVKeyCacheDoc,
				"CacheQuery":      KVKeyCacheQuery,
				"SessionWeb":      KVKeySessionWeb,
				"OperatorBind":    KVKeySessionOperatorBind,
				"WebBind":         KVKeySessionWebBind,
				"UserOperators":   KVKeyUserOperators,
				"UserWebSessions": KVKeyUserWebSessions,
				"UserMemories":    KVKeyUserMemories,
			},
			SessionType: map[string]string{
				"Web":      KVSessionTypeWeb,
				"Operator": KVSessionTypeOperator,
			},
		},
		Paths: PathsSnapshot{
			Infra: map[string]string{
				"db_path":                Paths.Infra.DbPath,
				"pki_dir":                Paths.Infra.PkiDir,
				"secrets_dir":            Paths.Infra.SecretsDir,
				"ca_cert_path":           Paths.Infra.CaCertPath,
				"app_cert_dir":           Paths.Infra.AppCertDir,
				"docs_dir":               Paths.Infra.DocsDir,
				"protocol_dir":           Paths.Infra.ProtocolDir,
				"protocol_constants_dir": Paths.Infra.ProtocolConstantsDir,
				"protocol_models_dir":    Paths.Infra.ProtocolModelsDir,
				"ssh_config_path":        Paths.Infra.SshConfigPath,
			},
			G8ee: map[string]string{
				"app_dir":    "/app/services/g8ee",
				"config_dir": "/app/services/g8ee/config",
				"tests_dir":  "/app/services/g8ee/tests",
				"cert_name":  "g8ee",
			},
			Ports: map[string]int{
				"operator_https":           Ports.OperatorHttps,
				"operator_bootstrap_https": Ports.OperatorBootstrapHttps,
				"operator_public_https":    Ports.OperatorPublicHttps,
				"g8ee_https":               Ports.G8eeHttps,
				"openclaw_gateway":         Ports.OpenclawGateway,
			},
			Host: "localhost",
		},
		Ports: map[string]int{
			"OperatorHttps":          Ports.OperatorHttps,
			"OperatorBootstrapHttps": Ports.OperatorBootstrapHttps,
			"OperatorPublicHttps":    Ports.OperatorPublicHttps,
			"G8eeHttps":              Ports.G8eeHttps,
			"OpenclawGateway":        Ports.OpenclawGateway,
		},
		EnvVars: map[string]string{
			"OperatorHTTPSPort":          string(EnvVar.OperatorHTTPSPort),
			"OperatorBootstrapHTTPSPort": string(EnvVar.OperatorBootstrapHTTPSPort),
			"OperatorPublicHTTPSPort":    string(EnvVar.OperatorPublicHTTPSPort),
			"OperatorWSSPort":            string(EnvVar.OperatorWSSPort),
			"G8EEHTTPSPort":              string(EnvVar.G8EEHTTPSPort),
			"PIDDir":                     string(EnvVar.PIDDir),
			"LogDir":                     string(EnvVar.LogDir),
			"OperatorPIDFile":            string(EnvVar.OperatorPIDFile),
			"OperatorLogFile":            string(EnvVar.OperatorLogFile),
			"G8EEPIDFile":                string(EnvVar.G8EEPIDFile),
			"G8EELogFile":                string(EnvVar.G8EELogFile),
			"LogMaxBackups":              string(EnvVar.LogMaxBackups),
			"LLMMaxTokens":               string(EnvVar.LLMMaxTokens),
			"LLMCommandGenEnabled":       string(EnvVar.LLMCommandGenEnabled),
			"LLMCommandGenAuditor":       string(EnvVar.LLMCommandGenAuditor),
			"LLMCommandGenPasses":        string(EnvVar.LLMCommandGenPasses),
			"SessionEncryptionKey":       string(EnvVar.SessionEncryptionKey),
			"InternalAPIKey":             string(EnvVar.InternalAPIKey),
			"AllowedOrigins":             string(EnvVar.AllowedOrigins),
			"PasskeyRPName":              string(EnvVar.PasskeyRPName),
			"PasskeyRPID":                string(EnvVar.PasskeyRPID),
			"PasskeyOrigin":              string(EnvVar.PasskeyOrigin),
			"VertexSearchEnabled":        string(EnvVar.VertexSearchEnabled),
			"VertexSearchProjectID":      string(EnvVar.VertexSearchProjectID),
			"VertexSearchEngineID":       string(EnvVar.VertexSearchEngineID),
			"VertexSearchLocation":       string(EnvVar.VertexSearchLocation),
			"VertexSearchAPIKey":         string(EnvVar.VertexSearchAPIKey),
			"GoogleSearchEnabled":        string(EnvVar.GoogleSearchEnabled),
			"GoogleSearchAPIKey":         string(EnvVar.GoogleSearchAPIKey),
			"GoogleSearchEngineID":       string(EnvVar.GoogleSearchEngineID),
			"OperatorURL":                string(EnvVar.OperatorURL),
			"OperatorPubSubURL":          string(EnvVar.OperatorPubSubURL),
			"OperatorBlobURL":            string(EnvVar.OperatorBlobURL),
			"DockerGID":                  string(EnvVar.DockerGID),
			"SessionTTL":                 string(EnvVar.SessionTTL),
			"AbsoluteSessionTimeout":     string(EnvVar.AbsoluteSessionTimeout),
			"Environment":                string(EnvVar.Environment),
			"UploadPath":                 string(EnvVar.UploadPath),
			"DocsDir":                    string(EnvVar.DocsDir),
			"G8EEURL":                    string(EnvVar.G8EEURL),
			"DashboardURL":               string(EnvVar.DashboardURL),
			"OperatorEndpoint":           string(EnvVar.OperatorEndpoint),
			"EnableCommandWhitelisting":  string(EnvVar.EnableCommandWhitelisting),
			"EnableCommandBlacklisting":  string(EnvVar.EnableCommandBlacklisting),
			"PKIDir":                     string(EnvVar.PKIDir),
			"SecretsDir":                 string(EnvVar.SecretsDir),
			"PubSubCACert":               string(EnvVar.PubSubCACert),
			"SSLCertFile":                string(EnvVar.SSLCertFile),
			"OperatorAPIKey":             string(EnvVar.OperatorAPIKey),
			"OperatorSessionID":          string(EnvVar.OperatorSessionID),
			"InternalAuthToken":          string(EnvVar.InternalAuthToken),
			"DeviceToken":                string(EnvVar.DeviceToken),
			"LogLevel":                   string(EnvVar.LogLevel),
			"DataDir":                    string(EnvVar.DataDir),
			"LocalStoreEnabled":          string(EnvVar.LocalStoreEnabled),
			"LocalDBPath":                string(EnvVar.LocalDBPath),
			"LocalStoreMaxSizeMB":        string(EnvVar.LocalStoreMaxSizeMB),
			"LocalStoreRetentionDays":    string(EnvVar.LocalStoreRetentionDays),
			"IPService":                  string(EnvVar.IPService),
			"IPResolver":                 string(EnvVar.IPResolver),
			"OpenClawGatewayToken":       string(EnvVar.OpenClawGatewayToken),
			"Shell":                      string(EnvVar.Shell),
			"Lang":                       string(EnvVar.Lang),
			"Term":                       string(EnvVar.Term),
			"TZ":                         string(EnvVar.TZ),
			"Path":                       string(EnvVar.Path),
			"SSHAuthSock":                string(EnvVar.SSHAuthSock),
			"User":                       string(EnvVar.User),
			"Username":                   string(EnvVar.Username),
			"LogName":                    string(EnvVar.LogName),
			"OperatorID":                 string(EnvVar.OperatorID),
			"UserID":                     string(EnvVar.UserID),
			"CLISessionID":               string(EnvVar.CLISessionID),
			"ProtocolDir":                string(EnvVar.ProtocolDir),
			"TestTmpDir":                 string(EnvVar.TestTmpDir),
			"TestOperatorPubSubURL":      string(EnvVar.TestOperatorPubSubURL),
			"StrictConstantsLint":        string(EnvVar.StrictConstantsLint),
			"RuntimeDir":                 string(EnvVar.RuntimeDir),
			"SSHConfigPath":              string(EnvVar.SSHConfigPath),
			"ProjectRoot":                string(EnvVar.ProjectRoot),
			"TestLLMAssistantProvider":   string(EnvVar.TestLLMAssistantProvider),
			"TestLLMAssistantModel":      string(EnvVar.TestLLMAssistantModel),
			"TestLLMLiteProvider":        string(EnvVar.TestLLMLiteProvider),
			"TestLLMLiteModel":           string(EnvVar.TestLLMLiteModel),
			"TestLLMAssistantAPIKey":     string(EnvVar.TestLLMAssistantAPIKey),
			"TestLLMLiteAPIKey":          string(EnvVar.TestLLMLiteAPIKey),
			"TestLLMMaxTokens":           string(EnvVar.TestLLMMaxTokens),
			"AuditorHMACKey":             string(EnvVar.AuditorHMACKey),
			"TestLLMPrimaryProvider":     string(EnvVar.TestLLMPrimaryProvider),
			"TestLLMPrimaryAPIKey":       string(EnvVar.TestLLMPrimaryAPIKey),
			"TestLLMPrimaryModel":        string(EnvVar.TestLLMPrimaryModel),
			"TestLLMPrimaryEndpoint":     string(EnvVar.TestLLMPrimaryEndpoint),
			"TestLLMAssistantEndpoint":   string(EnvVar.TestLLMAssistantEndpoint),
			"TestLLMLiteEndpoint":        string(EnvVar.TestLLMLiteEndpoint),
		},
		// Events and Status are large files; emit flat maps for now since Python models use extra="allow"
		// These will be refined to full Entry structures in a follow-up
		Events: map[string]Entry{
			"AppCaseCreated":                       {Value: string(EventAppCaseCreated), GoConst: "EventAppCaseCreated", PythonConst: "APP_CASE_CREATED"},
			"AppCaseUpdated":                       {Value: string(EventAppCaseUpdated), GoConst: "EventAppCaseUpdated", PythonConst: "APP_CASE_UPDATED"},
			"AppCaseAssigned":                      {Value: string(EventAppCaseAssigned), GoConst: "EventAppCaseAssigned", PythonConst: "APP_CASE_ASSIGNED"},
			"AppCaseEscalated":                     {Value: string(EventAppCaseEscalated), GoConst: "EventAppCaseEscalated", PythonConst: "APP_CASE_ESCALATED"},
			"AppCaseResolved":                      {Value: string(EventAppCaseResolved), GoConst: "EventAppCaseResolved", PythonConst: "APP_CASE_RESOLVED"},
			"AppCaseClosed":                        {Value: string(EventAppCaseClosed), GoConst: "EventAppCaseClosed", PythonConst: "APP_CASE_CLOSED"},
			"AppTaskCreated":                       {Value: string(EventAppTaskCreated), GoConst: "EventAppTaskCreated", PythonConst: "APP_TASK_CREATED"},
			"AppTaskUpdated":                       {Value: string(EventAppTaskUpdated), GoConst: "EventAppTaskUpdated", PythonConst: "APP_TASK_UPDATED"},
			"AppInvestigationCreated":              {Value: string(EventAppInvestigationCreated), GoConst: "EventAppInvestigationCreated", PythonConst: "APP_INVESTIGATION_CREATED"},
			"OperatorCommandRequested":             {Value: string(EventOperatorCommandRequested), GoConst: "EventOperatorCommandRequested", PythonConst: "OPERATOR_COMMAND_REQUESTED"},
			"OperatorCommandCompleted":             {Value: string(EventOperatorCommandCompleted), GoConst: "EventOperatorCommandCompleted", PythonConst: "OPERATOR_COMMAND_COMPLETED"},
			"OperatorCommandFailed":                {Value: string(EventOperatorCommandFailed), GoConst: "EventOperatorCommandFailed", PythonConst: "OPERATOR_COMMAND_FAILED"},
			"OperatorFileEditRequested":            {Value: string(EventOperatorFileEditRequested), GoConst: "EventOperatorFileEditRequested", PythonConst: "OPERATOR_FILE_EDIT_REQUESTED"},
			"OperatorFileEditCompleted":            {Value: string(EventOperatorFileEditCompleted), GoConst: "EventOperatorFileEditCompleted", PythonConst: "OPERATOR_FILE_EDIT_COMPLETED"},
			"OperatorHeartbeatSent":                {Value: string(EventOperatorHeartbeatSent), GoConst: "EventOperatorHeartbeatSent", PythonConst: "OPERATOR_HEARTBEAT_SENT"},
			"OperatorBound":                        {Value: string(EventOperatorBound), GoConst: "EventOperatorBound", PythonConst: "OPERATOR_BOUND"},
			"OperatorUnbound":                      {Value: string(EventOperatorUnbound), GoConst: "EventOperatorUnbound", PythonConst: "OPERATOR_UNBOUND"},
			"OperatorReputationCommitmentCreated":  {Value: string(EventOperatorReputationCommitmentCreated), GoConst: "EventOperatorReputationCommitmentCreated", PythonConst: "OPERATOR_REPUTATION_COMMITMENT_CREATED"},
			"OperatorReputationCommitmentVerified": {Value: string(EventOperatorReputationCommitmentVerified), GoConst: "EventOperatorReputationCommitmentVerified", PythonConst: "OPERATOR_REPUTATION_COMMITMENT_VERIFIED"},
			"OperatorReputationCommitmentFailed":   {Value: string(EventOperatorReputationCommitmentFailed), GoConst: "EventOperatorReputationCommitmentFailed", PythonConst: "OPERATOR_REPUTATION_COMMITMENT_FAILED"},
			"OperatorReputationStateUpdated":       {Value: string(EventOperatorReputationStateUpdated), GoConst: "EventOperatorReputationStateUpdated", PythonConst: "OPERATOR_REPUTATION_STATE_UPDATED"},
			"OperatorReputationSlashTier1":         {Value: string(EventOperatorReputationSlashTier1), GoConst: "EventOperatorReputationSlashTier1", PythonConst: "OPERATOR_REPUTATION_SLASH_TIER1"},
			"OperatorReputationSlashTier2":         {Value: string(EventOperatorReputationSlashTier2), GoConst: "EventOperatorReputationSlashTier2", PythonConst: "OPERATOR_REPUTATION_SLASH_TIER2"},
			"OperatorReputationSlashTier3":         {Value: string(EventOperatorReputationSlashTier3), GoConst: "EventOperatorReputationSlashTier3", PythonConst: "OPERATOR_REPUTATION_SLASH_TIER3"},
		},
		Status: StatusSnapshot{
			AttachmentType: map[string]Entry{
				"pdf":   {Value: string(AttachmentTypePdf), GoConst: "AttachmentTypePdf", PythonConst: "PDF"},
				"image": {Value: string(AttachmentTypeImage), GoConst: "AttachmentTypeImage", PythonConst: "IMAGE"},
				"text":  {Value: string(AttachmentTypeText), GoConst: "AttachmentTypeText", PythonConst: "TEXT"},
				"other": {Value: string(AttachmentTypeOther), GoConst: "AttachmentTypeOther", PythonConst: "OTHER"},
			},
			UserRole: map[string]Entry{
				"user":  {Value: string(UserRoleUser), GoConst: "UserRoleUser", PythonConst: "USER"},
				"admin": {Value: string(UserRoleAdmin), GoConst: "UserRoleAdmin", PythonConst: "ADMIN"},
			},
			UserStatus: map[string]Entry{
				"active":   {Value: string(UserStatusActive), GoConst: "UserStatusActive", PythonConst: "ACTIVE"},
				"disabled": {Value: string(UserStatusDisabled), GoConst: "UserStatusDisabled", PythonConst: "DISABLED"},
			},
			OperatorStatus: map[string]Entry{
				"available": {Value: string(OperatorStatusAvailable), GoConst: "OperatorStatusAvailable", PythonConst: "AVAILABLE"},
				"offline":   {Value: string(OperatorStatusOffline), GoConst: "OperatorStatusOffline", PythonConst: "OFFLINE"},
			},
			ExecutionStatus: map[string]Entry{
				"pending":   {Value: string(ExecutionStatusPending), GoConst: "ExecutionStatusPending", PythonConst: "PENDING"},
				"executing": {Value: string(ExecutionStatusExecuting), GoConst: "ExecutionStatusExecuting", PythonConst: "EXECUTING"},
				"completed": {Value: string(ExecutionStatusCompleted), GoConst: "ExecutionStatusCompleted", PythonConst: "COMPLETED"},
				"failed":    {Value: string(ExecutionStatusFailed), GoConst: "ExecutionStatusFailed", PythonConst: "FAILED"},
			},
			TribunalOutcome: map[string]Entry{
				"consensus":           {Value: string(TribunalOutcomeConsensus), GoConst: "TribunalOutcomeConsensus", PythonConst: "CONSENSUS"},
				"verified":            {Value: string(TribunalOutcomeVerified), GoConst: "TribunalOutcomeVerified", PythonConst: "VERIFIED"},
				"verification_failed": {Value: string(TribunalOutcomeVerificationFailed), GoConst: "TribunalOutcomeVerificationFailed", PythonConst: "VERIFICATION_FAILED"},
				"consensus_failed":    {Value: string(TribunalOutcomeConsensusFailed), GoConst: "TribunalOutcomeConsensusFailed", PythonConst: "CONSENSUS_FAILED"},
			},
			ApprovalErrorType: map[string]Entry{
				"approval.publish.failure":  {Value: string(ApprovalErrorTypeApprovalPublishFailure), GoConst: "ApprovalErrorTypeApprovalPublishFailure", PythonConst: "APPROVAL_PUBLISH_FAILURE"},
				"approval.exception":        {Value: string(ApprovalErrorTypeApprovalException), GoConst: "ApprovalErrorTypeApprovalException", PythonConst: "APPROVAL_EXCEPTION"},
				"approval.timeout":          {Value: string(ApprovalErrorTypeApprovalTimeout), GoConst: "ApprovalErrorTypeApprovalTimeout", PythonConst: "APPROVAL_TIMEOUT"},
				"invalid.intent":            {Value: string(ApprovalErrorTypeInvalidIntent), GoConst: "ApprovalErrorTypeInvalidIntent", PythonConst: "INVALID_INTENT"},
				"intent.approval.exception": {Value: string(ApprovalErrorTypeIntentApprovalException), GoConst: "ApprovalErrorTypeIntentApprovalException", PythonConst: "INTENT_APPROVAL_EXCEPTION"},
			},
			LlmModels: map[string]Entry{
				"llamacpp.gemma4.e2b": {Value: string(LLMModelsLlamacppGemma4E2b), GoConst: "LLMModelsLlamacppGemma4E2b", PythonConst: "LLAMACPP_GEMMA4_E2B"},
			},
		},
		Senders: map[string]Entry{
			"UserChat":     {Value: SourceUserChat, GoConst: "SourceUserChat", PythonConst: "USER_CHAT"},
			"UserTerminal": {Value: SourceUserTerminal, GoConst: "SourceUserTerminal", PythonConst: "USER_TERMINAL"},
			"AiPrimary":    {Value: SourceAiPrimary, GoConst: "SourceAiPrimary", PythonConst: "AI_PRIMARY"},
			"AiAssistant":  {Value: SourceAiAssistant, GoConst: "SourceAiAssistant", PythonConst: "AI_ASSISTANT"},
			"AiTriage":     {Value: SourceAiTriage, GoConst: "SourceAiTriage", PythonConst: "AI_TRIAGE"},
			"System":       {Value: SourceSystem, GoConst: "SourceSystem", PythonConst: "SYSTEM"},
			"TypeText":     {Value: MessageTypeText, GoConst: "MessageTypeText", PythonConst: "TYPE_TEXT"},
			"TypeCode":     {Value: MessageTypeCode, GoConst: "MessageTypeCode", PythonConst: "TYPE_CODE"},
			"TypeCall":     {Value: MessageTypeCall, GoConst: "MessageTypeCall", PythonConst: "TYPE_CALL"},
			"TypeResult":   {Value: MessageTypeResult, GoConst: "MessageTypeResult", PythonConst: "TYPE_RESULT"},
			"TypeError":    {Value: MessageTypeError, GoConst: "MessageTypeError", PythonConst: "TYPE_ERROR"},
			"TypeThinking": {Value: MessageTypeThinking, GoConst: "MessageTypeThinking", PythonConst: "TYPE_THINKING"},
		},
		PubSub: map[string]Entry{
			"FieldAction":  {Value: PubSubFieldAction, GoConst: "PubSubFieldAction", PythonConst: "FIELD_ACTION"},
			"FieldChannel": {Value: PubSubFieldChannel, GoConst: "PubSubFieldChannel", PythonConst: "FIELD_CHANNEL"},
			"FieldData":    {Value: PubSubFieldData, GoConst: "PubSubFieldData", PythonConst: "FIELD_DATA"},
			"FieldMessage": {Value: PubSubFieldMessage, GoConst: "PubSubFieldMessage", PythonConst: "FIELD_MESSAGE"},
			"FieldPattern": {Value: PubSubFieldPattern, GoConst: "PubSubFieldPattern", PythonConst: "FIELD_PATTERN"},
			"FieldType":    {Value: PubSubFieldType, GoConst: "PubSubFieldType", PythonConst: "FIELD_TYPE"},
			"FieldSender":  {Value: PubSubFieldSender, GoConst: "PubSubFieldSender", PythonConst: "FIELD_SENDER"},
		},
		Prompts: map[string]Entry{
			"AgentModeG8eBound":           {Value: AgentModeG8eBound, GoConst: "AgentModeG8eBound", PythonConst: "G8E_BOUND"},
			"AgentModeG8eNotBound":        {Value: AgentModeG8eNotBound, GoConst: "AgentModeG8eNotBound", PythonConst: "G8E_NOT_BOUND"},
			"AgentModeCloudOperatorBound": {Value: AgentModeCloudOperatorBound, GoConst: "AgentModeCloudOperatorBound", PythonConst: "CLOUD_OPERATOR_BOUND"},
			"SectionIdentity":             {Value: PromptSectionIdentity, GoConst: "PromptSectionIdentity", PythonConst: "IDENTITY"},
			"SectionSafety":               {Value: PromptSectionSafety, GoConst: "PromptSectionSafety", PythonConst: "SAFETY"},
			"SectionLoyalty":              {Value: PromptSectionLoyalty, GoConst: "PromptSectionLoyalty", PythonConst: "LOYALTY"},
			"SectionDissent":              {Value: PromptSectionDissent, GoConst: "PromptSectionDissent", PythonConst: "DISSENT"},
			"SectionCapabilities":         {Value: PromptSectionCapabilities, GoConst: "PromptSectionCapabilities", PythonConst: "CAPABILITIES"},
			"SectionExecution":            {Value: PromptSectionExecution, GoConst: "PromptSectionExecution", PythonConst: "EXECUTION"},
			"SectionTools":                {Value: PromptSectionTools, GoConst: "PromptSectionTools", PythonConst: "TOOLS"},
			"SectionDocs":                 {Value: PromptSectionDocs, GoConst: "PromptSectionDocs", PythonConst: "DOCS"},
			"SectionSystemContext":        {Value: PromptSectionSystemContext, GoConst: "PromptSectionSystemContext", PythonConst: "SYSTEM_CONTEXT"},
			"SectionSentinelMode":         {Value: PromptSectionSentinelMode, GoConst: "PromptSectionSentinelMode", PythonConst: "SENTINEL_MODE"},
			"SectionTriageContext":        {Value: PromptSectionTriageContext, GoConst: "PromptSectionTriageContext", PythonConst: "TRIAGE_CONTEXT"},
			"SectionInvestigationContext": {Value: PromptSectionInvestigationContext, GoConst: "PromptSectionInvestigationContext", PythonConst: "INVESTIGATION_CONTEXT"},
			"SectionResponseConstraints":  {Value: PromptSectionResponseConstraints, GoConst: "PromptSectionResponseConstraints", PythonConst: "RESPONSE_CONSTRAINTS"},
			"SectionLearnedContext":       {Value: PromptSectionLearnedContext, GoConst: "PromptSectionLearnedContext", PythonConst: "LEARNED_CONTEXT"},
			"SectionAgentPersona":         {Value: PromptSectionAgentPersona, GoConst: "PromptSectionAgentPersona", PythonConst: "AGENT_PERSONA"},
		},
		Platform: map[string]Entry{
			"UsageUpdated":             {Value: PlatformUsageUpdated, GoConst: "PlatformUsageUpdated", PythonConst: "USAGE_UPDATED"},
			"Notification":             {Value: PlatformNotification, GoConst: "PlatformNotification", PythonConst: "NOTIFICATION"},
			"AuthLoginRequested":       {Value: PlatformAuthLoginRequested, GoConst: "PlatformAuthLoginRequested", PythonConst: "AUTH_LOGIN_REQUESTED"},
			"AuthLoginSucceeded":       {Value: PlatformAuthLoginSucceeded, GoConst: "PlatformAuthLoginSucceeded", PythonConst: "AUTH_LOGIN_SUCCEEDED"},
			"AuthLoginFailed":          {Value: PlatformAuthLoginFailed, GoConst: "PlatformAuthLoginFailed", PythonConst: "AUTH_LOGIN_FAILED"},
			"AuthLogoutRequested":      {Value: PlatformAuthLogoutRequested, GoConst: "PlatformAuthLogoutRequested", PythonConst: "AUTH_LOGOUT_REQUESTED"},
			"AuthLogoutSucceeded":      {Value: PlatformAuthLogoutSucceeded, GoConst: "PlatformAuthLogoutSucceeded", PythonConst: "AUTH_LOGOUT_SUCCEEDED"},
			"AuthLogoutFailed":         {Value: PlatformAuthLogoutFailed, GoConst: "PlatformAuthLogoutFailed", PythonConst: "AUTH_LOGOUT_FAILED"},
			"SseKeepaliveSent":         {Value: PlatformSseKeepaliveSent, GoConst: "PlatformSseKeepaliveSent", PythonConst: "SSE_KEEPALIVE_SENT"},
			"SseConnectionEstablished": {Value: PlatformSseConnectionEstablished, GoConst: "PlatformSseConnectionEstablished", PythonConst: "SSE_CONNECTION_ESTABLISHED"},
			"SseConnectionOpened":      {Value: PlatformSseConnectionOpened, GoConst: "PlatformSseConnectionOpened", PythonConst: "SSE_CONNECTION_OPENED"},
			"SseConnectionClosed":      {Value: PlatformSseConnectionClosed, GoConst: "PlatformSseConnectionClosed", PythonConst: "SSE_CONNECTION_CLOSED"},
			"TerminalOpened":           {Value: PlatformTerminalOpened, GoConst: "PlatformTerminalOpened", PythonConst: "TERMINAL_OPENED"},
			"TerminalClosed":           {Value: PlatformTerminalClosed, GoConst: "PlatformTerminalClosed", PythonConst: "TERMINAL_CLOSED"},
			"SentinelModeChanged":      {Value: PlatformSentinelModeChanged, GoConst: "PlatformSentinelModeChanged", PythonConst: "SENTINEL_MODE_CHANGED"},
			"TelemetryHealthReported":  {Value: PlatformTelemetryHealthReported, GoConst: "PlatformTelemetryHealthReported", PythonConst: "TELEMETRY_HEALTH_REPORTED"},
			"ConsoleLogEntryReceived":  {Value: PlatformConsoleLogEntryReceived, GoConst: "PlatformConsoleLogEntryReceived", PythonConst: "CONSOLE_LOG_ENTRY_RECEIVED"},
		},
		Agents: map[string]string{
			"TriageComplexitySimple":                  TriageComplexitySimple,
			"TriageComplexityComplex":                 TriageComplexityComplex,
			"TriageConfidenceHigh":                    TriageConfidenceHigh,
			"TriageConfidenceLow":                     TriageConfidenceLow,
			"TriageIntentInformation":                 TriageIntentInformation,
			"TriageIntentAction":                      TriageIntentAction,
			"TriageIntentUnknown":                     TriageIntentUnknown,
			"TriagePostureNormal":                     TriagePostureNormal,
			"TriagePostureEscalated":                  TriagePostureEscalated,
			"TriagePostureAdversarial":                TriagePostureAdversarial,
			"TriagePostureConfused":                   TriagePostureConfused,
			"AgentNameSage":                           AgentNameSage,
			"AgentNameDash":                           AgentNameDash,
			"TribunalMemberAxiom":                     TribunalMemberAxiom,
			"TribunalMemberConcord":                   TribunalMemberConcord,
			"TribunalMemberVariance":                  TribunalMemberVariance,
			"TribunalMemberPragma":                    TribunalMemberPragma,
			"TribunalMemberNemesis":                   TribunalMemberNemesis,
			"TribunalAuditorReasonOk":                 TribunalAuditorReasonOk,
			"TribunalAuditorReasonEmptyResponse":      TribunalAuditorReasonEmptyResponse,
			"TribunalAuditorReasonNoValidRevision":    TribunalAuditorReasonNoValidRevision,
			"TribunalAuditorReasonAuditorError":       TribunalAuditorReasonAuditorError,
			"TribunalAuditorReasonSwappedToDissenter": TribunalAuditorReasonSwappedToDissenter,
			"TribunalAuditorReasonRevisedFromDissent": TribunalAuditorReasonRevisedFromDissent,
			"TribunalAuditorReasonWhitelistViolation": TribunalAuditorReasonWhitelistViolation,
			"TribunalTieBreakReasonShortest":          TribunalTieBreakReasonShortest,
			"TribunalTieBreakReasonExcludedNemesis":   TribunalTieBreakReasonExcludedNemesis,
			"TribunalTieBreakReasonAlphabetical":      TribunalTieBreakReasonAlphabetical,
		},
		Timestamp: map[string]string{
			"FormatRFC3339": TimestampFormatRFC3339,
		},
		ApiPaths: ApiPaths,
	}
}
