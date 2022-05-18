#!/usr/bin/env bash

# NOTE! THIS IS INTENDED TO BE RUN ON YOUR LOCAL SYSTEM, NOT IN THE DEV CONTAINER.
# The dev container does not contain the AWS CLI, and the QUEUE_URL is pointed at
# localhost (which works for your system) instead of localstack (the address from
# the dev container)
set +e

export AWS_ACCESS_KEY_ID=fake
export AWS_SECRET_ACCESS_KEY=fake

QUEUE_URL=http://localhost:4566/000000000000/development-artifacts.fifo
echo "Polling $QUEUE_URL for development artifacts..."

while (true); do
    MESSAGES=$(aws --region us-east-1 --endpoint-url http://localhost:4566 sqs receive-message --queue-url "$QUEUE_URL" --wait-time-seconds 10 --max-number-of-messages 1)
    if [[ "$MESSAGES" != "" ]]; then
        MESSAGE=$(echo $MESSAGES | jq '.Messages[]' -r)
        RECEIPT=$(echo $MESSAGE | jq '.ReceiptHandle' -r)
        BODY=$(echo $MESSAGE | jq '.Body' -r)
        PAYLOAD=$(echo $BODY | jq '.Message | fromjson' -r)
        echo ""
        echo "Receipt: $RECEIPT"
        echo "Got Artifact:"
        echo "$PAYLOAD"
        aws --region us-east-1 --endpoint-url http://localhost:4566 sqs delete-message --queue-url "$QUEUE_URL" --receipt-handle "$RECEIPT"
    fi
done
