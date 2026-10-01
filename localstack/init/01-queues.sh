#!/bin/sh
set -eu

echo "Creating Jungle Gaming SQS queues..."

DLQ_URL="$(
    awslocal sqs create-queue \
        --queue-name wager-transactions-dlq.fifo \
        --attributes FifoQueue=true,ContentBasedDeduplication=false \
        --query QueueUrl \
        --output text
)"

DLQ_ARN="$(
    awslocal sqs get-queue-attributes \
        --queue-url "$DLQ_URL" \
        --attribute-names QueueArn \
        --query 'Attributes.QueueArn' \
        --output text
)"

TRANSACTIONS_QUEUE_URL="$(
    awslocal sqs create-queue \
        --queue-name wager-transactions.fifo \
        --attributes FifoQueue=true,ContentBasedDeduplication=false,VisibilityTimeout=30 \
        --query QueueUrl \
        --output text
)"

cat > /tmp/redrive-policy.json <<EOF
{
  "RedrivePolicy": "{\"deadLetterTargetArn\":\"$DLQ_ARN\",\"maxReceiveCount\":\"5\"}"
}
EOF

awslocal sqs set-queue-attributes \
    --queue-url "$TRANSACTIONS_QUEUE_URL" \
    --attributes file:///tmp/redrive-policy.json

awslocal sqs create-queue \
    --queue-name wager-events.fifo \
    --attributes FifoQueue=true,ContentBasedDeduplication=false

awslocal sqs create-queue \
    --queue-name outbox-publisher-integration.fifo \
    --attributes FifoQueue=true,ContentBasedDeduplication=false

echo "Jungle Gaming SQS queues created."