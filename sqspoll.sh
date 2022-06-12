#!/usr/bin/env bash

set +e

QUEUE_URL=http://localstack:4566/000000000000/massdriver-provider-test.fifo
echo "Polling $QUEUE_URL for development artifacts..."

while (true); do
    MESSAGES=$(aws --region us-east-1 --endpoint-url http://localstack:4566 sqs receive-message --queue-url "$QUEUE_URL" --wait-time-seconds 10 --max-number-of-messages 1)
    if [[ "$MESSAGES" != "" ]]; then
        MESSAGE=$(echo $MESSAGES | jq '.Messages[]' -r)
        RECEIPT=$(echo $MESSAGE | jq '.ReceiptHandle' -r)
        BODY=$(echo $MESSAGE | jq '.Body' -r)
        PAYLOAD=$(echo $BODY | jq '.Message | fromjson' -r)
        echo ""
        echo "Receipt: $RECEIPT"
        echo "Got Resource:"
        echo "$PAYLOAD"
        aws --region us-east-1 --endpoint-url http://localstack:4566 sqs delete-message --queue-url "$QUEUE_URL" --receipt-handle "$RECEIPT"
    fi
done
