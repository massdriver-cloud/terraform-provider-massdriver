provider "aws" {
  access_key                  = "mock_access_key"
  secret_key                  = "mock_secret_key"
  region                      = "us-east-1"
  skip_credentials_validation = true
  skip_metadata_api_check     = true
  skip_requesting_account_id  = true

  endpoints {
    apigateway     = "http://localstack:4568"
    cloudformation = "http://localstack:4568"
    cloudwatch     = "http://localstack:4568"
    dynamodb       = "http://localstack:4568"
    es             = "http://localstack:4568"
    firehose       = "http://localstack:4568"
    iam            = "http://localstack:4568"
    kinesis        = "http://localstack:4568"
    lambda         = "http://localstack:4568"
    route53        = "http://localstack:4568"
    redshift       = "http://localstack:4568"
    s3             = "http://localstack:4568"
    secretsmanager = "http://localstack:4568"
    ses            = "http://localstack:4568"
    sns            = "http://localstack:4568"
    sqs            = "http://localstack:4568"
    ssm            = "http://localstack:4568"
    stepfunctions  = "http://localstack:4568"
    sts            = "http://localstack:4568"
  }
}

#resource "aws_sqs_queue" "artifacts" {
#  name       = "development-artifacts.fifo"
#  fifo_queue = true
#}

resource "aws_sns_topic" "artifacts" {
  name                        = "massdriver-provider-test.fifo"
  fifo_topic                  = true
  content_based_deduplication = true
}

#resource "aws_sns_topic_subscription" "artifacts" {
#  topic_arn = aws_sns_topic.artifacts.arn
#  protocol  = "sqs"
#  endpoint  = aws_sqs_queue.artifacts.arn
#}