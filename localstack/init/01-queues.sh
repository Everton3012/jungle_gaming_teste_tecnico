#!/bin/sh
set -eu

echo "Creating Jungle Gaming SQS queues..."

create_fifo_queue() {
    QUEUE_NAME="$1"

    awslocal sqs create-queue \
        --queue-name "$QUEUE_NAME" \
        --attributes FifoQueue=true,ContentBasedDeduplication=false \
        >/dev/null
}

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

TRANSACTION_QUEUE_URL="$(
    awslocal sqs create-queue \
        --queue-name wager-transactions.fifo \
        --attributes FifoQueue=true,ContentBasedDeduplication=false,VisibilityTimeout=30 \
        --query QueueUrl \
        --output text
)"

REDRIVE_POLICY="$(
    printf '{"deadLetterTargetArn":"%s","maxReceiveCount":"5"}' "$DLQ_ARN"
)"

ATTRIBUTES_FILE="/tmp/wager-redrive-policy.json"

printf '%s\n' \
    "{\"RedrivePolicy\":\"$(printf '%s' "$REDRIVE_POLICY" | sed 's/"/\\"/g')\"}" \
    > "$ATTRIBUTES_FILE"

awslocal sqs set-queue-attributes \
    --queue-url "$TRANSACTION_QUEUE_URL" \
    --attributes "file://$ATTRIBUTES_FILE"

create_fifo_queue "wager-events.fifo"
create_fifo_queue "outbox-publisher-integration.fifo"
create_fifo_queue "sqs-consumer-integration.fifo"
create_fifo_queue "wager-consumer-integration.fifo"
create_fifo_queue "fx-lifecycle-consumer.fifo"
create_fifo_queue "fx-lifecycle-outbox.fifo"

echo "Jungle Gaming SQS queues created."