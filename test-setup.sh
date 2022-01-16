# this is intended to be run on your local system where the AWS CLI is installed
aws --endpoint-url=http://localhost:4566 sqs create-queue --queue-name massdriver-provider-test.fifo --attributes FifoQueue=true
aws --endpoint-url=http://localhost:4566 sns create-topic --name massdriver-provider-test.fifo --attributes FifoTopic=true
aws --endpoint-url=http://localhost:4566 sns subscribe --topic-arn "arn:aws:sns:us-east-1:000000000000:massdriver-provider-test.fifo" --protocol sqs --notification-endpoint "http://localhost:4566/000000000000/massdriver-provider-test.fifo"

# you can check on the sqs message created with this:
aws --endpoint-url=http://localhost:4566 sqs receive-message --queue-url http://localhost:4566/000000000000/massdriver-provider-test.fifo